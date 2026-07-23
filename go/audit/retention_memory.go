package audit

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"time"
)

func (m *Memory) PlaceHold(_ context.Context, command PlaceHoldCommand) (LegalHold, error) {
	if !codePattern.MatchString(command.ID) {
		return LegalHold{}, &Error{Kind: ErrorHoldConflict, Field: "id", Message: "is invalid"}
	}
	if !codePattern.MatchString(string(command.Reason)) {
		return LegalHold{}, &Error{Kind: ErrorHoldConflict, Field: "reason", Message: "is invalid"}
	}
	if err := validateMaintenanceActor(command.Actor); err != nil {
		return LegalHold{}, err
	}
	selection, err := normalizeHoldSelection(command.Selection)
	if err != nil {
		return LegalHold{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, found := m.holds[command.ID]; found {
		if existing.ReleasedAt == nil && sameHold(existing, command, selection) {
			return cloneHold(existing), nil
		}
		return LegalHold{}, &Error{Kind: ErrorHoldConflict, Field: "id", Message: "was already used"}
	}
	hold := LegalHold{
		ID: command.ID, Selection: selection, Reason: command.Reason,
		PlacedBy: command.Actor, PlacedAt: m.clock.Now().UTC(),
	}
	m.holds[hold.ID] = hold
	return cloneHold(hold), nil
}

func (m *Memory) ReleaseHold(_ context.Context, command ReleaseHoldCommand) (LegalHold, error) {
	if !codePattern.MatchString(command.ID) || !codePattern.MatchString(string(command.Reason)) {
		return LegalHold{}, &Error{Kind: ErrorHoldConflict, Field: "command", Message: "is invalid"}
	}
	if err := validateMaintenanceActor(command.Actor); err != nil {
		return LegalHold{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	hold, found := m.holds[command.ID]
	if !found {
		return LegalHold{}, &Error{Kind: ErrorHoldConflict, Field: "id", Message: "does not exist"}
	}
	if hold.ReleasedAt != nil {
		if hold.ReleaseReason == command.Reason && hold.ReleasedBy != nil && *hold.ReleasedBy == command.Actor {
			return cloneHold(hold), nil
		}
		return LegalHold{}, &Error{Kind: ErrorHoldConflict, Field: "id", Message: "was already released"}
	}
	now := m.clock.Now().UTC()
	actor := command.Actor
	hold.ReleaseReason = command.Reason
	hold.ReleasedBy = &actor
	hold.ReleasedAt = &now
	m.holds[hold.ID] = hold
	return cloneHold(hold), nil
}

func (m *Memory) RunRetention(ctx context.Context, command RetentionCommand) (RetentionReceipt, error) {
	if !codePattern.MatchString(command.ID) {
		return RetentionReceipt{}, invalidAttempt("id", "is invalid")
	}
	if err := validateMaintenanceActor(command.Actor); err != nil {
		return RetentionReceipt{}, err
	}
	if command.AsOf.IsZero() {
		command.AsOf = m.clock.Now().UTC()
	} else {
		command.AsOf = command.AsOf.UTC()
	}
	if command.BatchLimit == 0 {
		command.BatchLimit = 1000
	}
	if command.BatchLimit < 1 || command.BatchLimit > 10000 {
		return RetentionReceipt{}, invalidAttempt("batch_limit", "must be between 1 and 10000")
	}

	m.mu.RLock()
	if receipt, found := m.retentions[command.ID]; found {
		m.mu.RUnlock()
		return cloneRetentionReceipt(receipt), nil
	}
	head, headDigest := m.head, m.headDigest
	candidates, archiveRequired := m.retentionCandidates(command.AsOf, command.BatchLimit)
	m.mu.RUnlock()

	ranges := sequenceRanges(candidates)
	var archive *ArchiveReceipt
	if len(candidates) > 0 && (archiveRequired || command.Archive != nil) {
		if command.Archive == nil {
			return RetentionReceipt{}, &Error{Kind: ErrorArchiveRequired, Field: "archive", Message: "is required by the retention class"}
		}
		var writtenDigest Digest
		descriptor := ArchiveDescriptor{
			RetentionID: command.ID, Instance: m.source.Instance, AsOf: command.AsOf,
			ExpectedCount: uint64(len(candidates)), ExpectedRanges: slices.Clone(ranges),
			DefinitionDigest: m.catalog.digest,
		}
		value, err := command.Archive.Put(ctx, descriptor, func(writer io.Writer) error {
			var writeErr error
			writtenDigest, writeErr = writeRetentionArchive(writer, candidates)
			return writeErr
		})
		if err != nil {
			return RetentionReceipt{}, &Error{Kind: ErrorArchiveRequired, Field: "archive", Message: "failed"}
		}
		if value.Reference == "" || value.Count != uint64(len(candidates)) || value.ContentDigest != writtenDigest {
			return RetentionReceipt{}, &Error{Kind: ErrorArchiveRequired, Field: "archive", Message: "receipt does not match written content"}
		}
		archive = &value
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if receipt, found := m.retentions[command.ID]; found {
		return cloneRetentionReceipt(receipt), nil
	}
	if m.head != head || m.headDigest != headDigest {
		return RetentionReceipt{}, &Error{Kind: ErrorUnavailable, Field: "journal", Message: "changed while retention archive was written"}
	}
	confirmed, _ := m.retentionCandidates(command.AsOf, command.BatchLimit)
	if !sameEventIDs(candidates, confirmed) {
		return RetentionReceipt{}, &Error{Kind: ErrorHoldConflict, Field: "selection", Message: "changed while retention archive was written"}
	}
	deleted := make(map[EventID]struct{}, len(candidates))
	for _, event := range candidates {
		deleted[event.ID] = struct{}{}
		receipt := m.byID[event.ID]
		receipt.event = Event{}
		receipt.purged = true
		m.byID[event.ID] = receipt
	}
	kept := m.events[:0]
	for _, event := range m.events {
		if _, remove := deleted[event.ID]; !remove {
			kept = append(kept, event)
		}
	}
	m.events = slices.Clip(kept)
	receipt := RetentionReceipt{
		ID: command.ID, AsOf: command.AsOf, Deleted: uint64(len(candidates)),
		Ranges: ranges, Archive: archive, CreatedAt: m.clock.Now().UTC(),
	}
	m.retentions[receipt.ID] = receipt
	return cloneRetentionReceipt(receipt), nil
}

func (m *Memory) retentionCandidates(asOf time.Time, limit int) ([]Event, bool) {
	candidates := make([]Event, 0, limit)
	archiveRequired := false
	for _, event := range m.events {
		retention := m.catalog.retention[event.RetentionClass]
		if event.RecordedAt.Add(retention.MinimumAge).After(asOf) || m.eventHeld(event) {
			continue
		}
		candidates = append(candidates, cloneEvent(event))
		archiveRequired = archiveRequired || retention.ArchiveBefore
		if len(candidates) == limit {
			break
		}
	}
	return candidates, archiveRequired
}

func (m *Memory) eventHeld(event Event) bool {
	for _, hold := range m.holds {
		if hold.ReleasedAt == nil && heldBy(hold.Selection, event) {
			return true
		}
	}
	return false
}

func sameHold(existing LegalHold, command PlaceHoldCommand, selection HoldSelection) bool {
	if existing.Reason != command.Reason || existing.PlacedBy != command.Actor {
		return false
	}
	left, _ := json.Marshal(existing.Selection)
	right, _ := json.Marshal(selection)
	return string(left) == string(right)
}

func sameEventIDs(left, right []Event) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID || left[index].Digest != right[index].Digest {
			return false
		}
	}
	return true
}

func cloneHold(in LegalHold) LegalHold {
	out := in
	out.Selection.EventIDs = slices.Clone(in.Selection.EventIDs)
	out.Selection.Query.Actions = slices.Clone(in.Selection.Query.Actions)
	out.Selection.Query.Outcomes = slices.Clone(in.Selection.Query.Outcomes)
	out.Selection.Query.RetentionClasses = slices.Clone(in.Selection.Query.RetentionClasses)
	if in.ReleasedBy != nil {
		value := *in.ReleasedBy
		out.ReleasedBy = &value
	}
	if in.ReleasedAt != nil {
		value := *in.ReleasedAt
		out.ReleasedAt = &value
	}
	return out
}

func cloneRetentionReceipt(in RetentionReceipt) RetentionReceipt {
	out := in
	out.Ranges = slices.Clone(in.Ranges)
	if in.Archive != nil {
		value := *in.Archive
		out.Archive = &value
	}
	return out
}
