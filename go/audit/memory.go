package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

type MemoryOptions struct {
	Clock  Clock
	Source Source
}

type memoryReceipt struct {
	fingerprint Digest
	event       Event
	purged      bool
}

type Memory struct {
	mu         sync.RWMutex
	catalog    *Catalog
	clock      Clock
	source     Source
	events     []Event
	byID       map[EventID]memoryReceipt
	holds      map[string]LegalHold
	retentions map[string]RetentionReceipt
	head       Sequence
	headDigest Digest
}

func NewMemory(catalog *Catalog, options MemoryOptions) (*Memory, error) {
	if catalog == nil {
		return nil, invalidDefinition("catalog", "is required")
	}
	if options.Clock == nil {
		options.Clock = ClockFunc(time.Now)
	}
	if options.Source.Service == "" || options.Source.Instance == "" {
		return nil, invalidDefinition("source", "service and instance are required")
	}
	return &Memory{
		catalog: catalog, clock: options.Clock, source: options.Source,
		byID: make(map[EventID]memoryReceipt), holds: make(map[string]LegalHold),
		retentions: make(map[string]RetentionReceipt),
	}, nil
}

func (m *Memory) Append(ctx context.Context, command Command) (Event, error) {
	events, err := m.AppendBatch(ctx, []Command{command})
	if err != nil {
		return Event{}, err
	}
	return events[0], nil
}

func (m *Memory) AppendBatch(_ context.Context, commands []Command) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(commands) == 0 || len(commands) > m.catalog.definition.MaxBatch {
		return nil, invalidAttempt("batch", "size is outside the compiled limit")
	}
	seen := make(map[EventID]struct{}, len(commands))
	replays := make([]Event, 0, len(commands))
	replayCount := 0
	for _, command := range commands {
		value := command.value
		if value.DefinitionDigest != m.catalog.digest || value.ID == "" {
			return nil, invalidAttempt("command", "was not prepared by this Catalog")
		}
		if _, exists := seen[value.ID]; exists {
			return nil, invalidAttempt("batch", "contains a duplicate event ID")
		}
		seen[value.ID] = struct{}{}
		if receipt, exists := m.byID[value.ID]; exists {
			if receipt.fingerprint != value.Fingerprint {
				return nil, &Error{Kind: ErrorIdempotencyConflict, Field: "id", Message: "was already used for different evidence"}
			}
			if receipt.purged {
				return nil, &Error{Kind: ErrorIdempotencyConflict, Field: "id", Message: "belongs to a purged audit event"}
			}
			replayCount++
			replays = append(replays, cloneEvent(receipt.event))
		}
	}
	if replayCount > 0 {
		if replayCount != len(commands) {
			return nil, &Error{Kind: ErrorIdempotencyConflict, Field: "batch", Message: "is only partially replayed"}
		}
		return replays, nil
	}
	recordedAt := m.clock.Now().UTC()
	next := make([]Event, 0, len(commands))
	previous := m.headDigest
	for index, command := range commands {
		value := command.value
		occurredAt := value.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = recordedAt
		}
		event := Event{
			ID: value.ID, EnvelopeVersion: 1, Sequence: m.head + Sequence(index+1),
			Source: m.source, Action: value.Action, Actor: value.Actor, Target: value.Target,
			Outcome: value.Outcome, Correlation: value.Correlation, Evidence: cloneEvidence(value.Evidence),
			RetentionClass: value.RetentionClass, DefinitionDigest: value.DefinitionDigest,
			OccurredAt: occurredAt.UTC(), RecordedAt: recordedAt, PreviousDigest: previous,
		}
		digest, err := eventDigest(event)
		if err != nil {
			return nil, &Error{Kind: ErrorUnavailable, Field: "event", Message: "cannot be encoded"}
		}
		event.Digest = digest
		previous = digest
		next = append(next, event)
	}
	for index, event := range next {
		m.events = append(m.events, event)
		m.byID[event.ID] = memoryReceipt{fingerprint: commands[index].value.Fingerprint, event: event}
	}
	m.head += Sequence(len(next))
	m.headDigest = previous
	return cloneEvents(next), nil
}

func (m *Memory) Query(_ context.Context, input Query) (Page, error) {
	query, filterDigest, err := normalizeQuery(input)
	if err != nil {
		return Page{}, err
	}
	before, err := decodeCursor(query.Before, filterDigest)
	if err != nil {
		return Page{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if before == 0 {
		before = m.head + 1
	}
	out := make([]Event, 0, query.Limit)
	var nextSequence Sequence
	for index := len(m.events) - 1; index >= 0; index-- {
		event := m.events[index]
		if event.Sequence >= before || !matchesQuery(event, query) {
			continue
		}
		if len(out) == query.Limit {
			nextSequence = out[len(out)-1].Sequence
			break
		}
		out = append(out, cloneEvent(event))
	}
	page := Page{Events: out}
	if nextSequence != 0 {
		page.NextCursor = encodeCursor(nextSequence, filterDigest)
	}
	return page, nil
}

func (m *Memory) Get(_ context.Context, id EventID) (Event, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	receipt, exists := m.byID[id]
	if !exists {
		return Event{}, false, nil
	}
	return cloneEvent(receipt.event), true, nil
}

func eventDigest(event Event) (Digest, error) {
	copy := event
	copy.Digest = ""
	raw, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return Digest(hex.EncodeToString(sum[:])), nil
}

func cloneEvent(in Event) Event {
	out := in
	out.Evidence = cloneEvidence(in.Evidence)
	return out
}

func cloneEvents(in []Event) []Event {
	out := make([]Event, len(in))
	for index := range in {
		out[index] = cloneEvent(in[index])
	}
	return out
}
