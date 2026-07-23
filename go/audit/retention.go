package audit

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"slices"
	"time"
)

type HoldSelection struct {
	EventIDs []EventID `json:"eventIds,omitempty"`
	Query    Query     `json:"query,omitempty"`
}

type PlaceHoldCommand struct {
	ID        string        `json:"id"`
	Selection HoldSelection `json:"selection"`
	Reason    ReasonCode    `json:"reason"`
	Actor     Actor         `json:"actor"`
}

type ReleaseHoldCommand struct {
	ID     string     `json:"id"`
	Reason ReasonCode `json:"reason"`
	Actor  Actor      `json:"actor"`
}

type LegalHold struct {
	ID            string        `json:"id"`
	Selection     HoldSelection `json:"selection"`
	Reason        ReasonCode    `json:"reason"`
	PlacedBy      Actor         `json:"placedBy"`
	PlacedAt      time.Time     `json:"placedAt"`
	ReleaseReason ReasonCode    `json:"releaseReason,omitempty"`
	ReleasedBy    *Actor        `json:"releasedBy,omitempty"`
	ReleasedAt    *time.Time    `json:"releasedAt,omitempty"`
}

type SequenceRange struct {
	First          Sequence `json:"first"`
	Last           Sequence `json:"last"`
	PreviousDigest Digest   `json:"previousDigest,omitempty"`
	LastDigest     Digest   `json:"lastDigest"`
}

type ArchiveDescriptor struct {
	RetentionID      string          `json:"retentionId"`
	Instance         string          `json:"instance"`
	AsOf             time.Time       `json:"asOf"`
	ExpectedCount    uint64          `json:"expectedCount"`
	ExpectedRanges   []SequenceRange `json:"expectedRanges"`
	DefinitionDigest Digest          `json:"definitionDigest"`
}

type ArchiveReceipt struct {
	Reference     string `json:"reference"`
	Count         uint64 `json:"count"`
	ContentDigest Digest `json:"contentDigest"`
}

type ArchiveSink interface {
	Put(context.Context, ArchiveDescriptor, func(io.Writer) error) (ArchiveReceipt, error)
}

type RetentionCommand struct {
	ID         string      `json:"id"`
	AsOf       time.Time   `json:"asOf"`
	BatchLimit int         `json:"batchLimit,omitempty"`
	Actor      Actor       `json:"actor"`
	Archive    ArchiveSink `json:"-"`
}

type RetentionReceipt struct {
	ID        string          `json:"id"`
	AsOf      time.Time       `json:"asOf"`
	Deleted   uint64          `json:"deleted"`
	Ranges    []SequenceRange `json:"ranges,omitempty"`
	Archive   *ArchiveReceipt `json:"archive,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
}

type Maintainer interface {
	PlaceHold(context.Context, PlaceHoldCommand) (LegalHold, error)
	ReleaseHold(context.Context, ReleaseHoldCommand) (LegalHold, error)
	RunRetention(context.Context, RetentionCommand) (RetentionReceipt, error)
	Verify(context.Context, VerifyRequest) (VerifyResult, error)
}

func normalizeHoldSelection(selection HoldSelection) (HoldSelection, error) {
	selection.EventIDs = slices.Clone(selection.EventIDs)
	slices.Sort(selection.EventIDs)
	selection.EventIDs = slices.Compact(selection.EventIDs)
	selection.Query.Before = ""
	selection.Query.Limit = 500
	query, _, err := normalizeQuery(selection.Query)
	if err != nil {
		return HoldSelection{}, &Error{Kind: ErrorHoldConflict, Field: "selection", Message: "is invalid"}
	}
	query.Limit = 0
	selection.Query = query
	if len(selection.EventIDs) == 0 && holdQueryEmpty(query) {
		return HoldSelection{}, &Error{Kind: ErrorHoldConflict, Field: "selection", Message: "must select events"}
	}
	return selection, nil
}

func holdQueryEmpty(query Query) bool {
	return len(query.Actions) == 0 && query.Actor == nil && query.Target == nil &&
		len(query.Outcomes) == 0 && len(query.RetentionClasses) == 0 &&
		query.RequestID == "" && query.TraceID == "" && query.CommandID == "" &&
		query.From.IsZero() && query.To.IsZero()
}

func heldBy(selection HoldSelection, event Event) bool {
	if slices.Contains(selection.EventIDs, event.ID) {
		return true
	}
	return !holdQueryEmpty(selection.Query) && matchesQuery(event, selection.Query)
}

func validateMaintenanceActor(actor Actor) error {
	if actor.Kind != ActorUser && actor.Kind != ActorService && actor.Kind != ActorSystem {
		return invalidAttempt("actor.kind", "must identify a user, service, or system")
	}
	if actor.ID == "" {
		return invalidAttempt("actor.id", "is required")
	}
	return nil
}

func sequenceRanges(events []Event) []SequenceRange {
	if len(events) == 0 {
		return nil
	}
	ranges := make([]SequenceRange, 0, len(events))
	current := SequenceRange{
		First: events[0].Sequence, Last: events[0].Sequence,
		PreviousDigest: events[0].PreviousDigest, LastDigest: events[0].Digest,
	}
	for _, event := range events[1:] {
		if event.Sequence == current.Last+1 {
			current.Last = event.Sequence
			current.LastDigest = event.Digest
			continue
		}
		ranges = append(ranges, current)
		current = SequenceRange{
			First: event.Sequence, Last: event.Sequence,
			PreviousDigest: event.PreviousDigest, LastDigest: event.Digest,
		}
	}
	return append(ranges, current)
}

func writeRetentionArchive(writer io.Writer, events []Event) (Digest, error) {
	hash := sha256.New()
	buffered := bufio.NewWriter(io.MultiWriter(writer, hash))
	encoder := json.NewEncoder(buffered)
	if err := encoder.Encode(map[string]any{"kind": "audit.retention.archive", "version": 1}); err != nil {
		return "", err
	}
	for _, event := range events {
		if err := encoder.Encode(struct {
			Kind  string `json:"kind"`
			Event Event  `json:"event"`
		}{"audit.event", event}); err != nil {
			return "", err
		}
	}
	if err := buffered.Flush(); err != nil {
		return "", err
	}
	return Digest(hex.EncodeToString(hash.Sum(nil))), nil
}
