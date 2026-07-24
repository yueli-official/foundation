package privacy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MemoryOptions struct {
	Clock func() time.Time
}

type Memory struct {
	catalog *Catalog
	clock   func() time.Time
	mu      sync.RWMutex

	commands          map[IdempotencyKey]memoryCommand
	events            []memoryEvidence
	retentionCommands map[IdempotencyKey]memoryRetentionCommand
	retentionItems    map[RetentionItemID]RetentionItem
}

type memoryCommand struct {
	fingerprint string
	kind        string
	consent     ConsentReceipt
	withdrawal  WithdrawalReceipt
	signal      SignalReceipt
}

type memoryEvidence struct {
	kind       string
	subject    SubjectRef
	purposes   []PurposeRef
	signal     SignalKey
	occurredAt time.Time
	recordedAt time.Time
	expiresAt  *time.Time
	id         ReceiptID
}

type memoryRetentionCommand struct {
	fingerprint string
	itemID      RetentionItemID
}

var _ Runtime = (*Memory)(nil)
var _ EvidenceLedger = (*Memory)(nil)
var _ RetentionLedger = (*Memory)(nil)

func NewMemory(catalog *Catalog, options MemoryOptions) (*Memory, error) {
	if catalog == nil {
		return nil, invalid("catalog", "is required")
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Memory{
		catalog: catalog, clock: clock,
		commands:          map[IdempotencyKey]memoryCommand{},
		retentionCommands: map[IdempotencyKey]memoryRetentionCommand{},
		retentionItems:    map[RetentionItemID]RetentionItem{},
	}, nil
}

func (memory *Memory) Evidence() EvidenceLedger   { return memory }
func (memory *Memory) Retention() RetentionLedger { return memory }

func (memory *Memory) Purpose(key PurposeKey) (Processing, error) {
	if memory == nil || memory.catalog == nil {
		return nil, invalid("runtime", "is required")
	}
	ref, exists := memory.catalog.active[key]
	if !exists {
		return nil, &Error{Kind: ErrorNotFound, Field: "purpose", Message: fmt.Sprintf("%q is not active", key)}
	}
	return &memoryProcessing{memory: memory, ref: ref}, nil
}

type memoryProcessing struct {
	memory *Memory
	ref    PurposeRef
}

func (processing *memoryProcessing) Ref() PurposeRef { return processing.ref }

func (processing *memoryProcessing) Decide(ctx context.Context, input DecisionInput) (ProcessingDecision, error) {
	if err := ctx.Err(); err != nil {
		return ProcessingDecision{}, err
	}
	return processing.memory.decide(processing.ref, input)
}

func (memory *Memory) decide(ref PurposeRef, input DecisionInput) (ProcessingDecision, error) {
	purpose, exists := memory.catalog.purposes[ref]
	if !exists {
		return ProcessingDecision{}, &Error{Kind: ErrorNotFound, Field: "purpose", Message: "is not defined"}
	}
	at := input.At
	if at.IsZero() {
		at = memory.clock()
	}
	at = at.UTC()
	decision := ProcessingDecision{
		Kind: DecisionDeny, Purpose: ref, Basis: purpose.Basis,
		Reasons: []ReasonCode{"purpose_unavailable"}, CatalogDigest: memory.catalog.digest, DecidedAt: at,
	}
	if (!purpose.EffectiveAt.IsZero() && at.Before(purpose.EffectiveAt)) ||
		(purpose.RetiredAt != nil && !at.Before(*purpose.RetiredAt)) {
		return decision, nil
	}
	signals := append([]ObservedSignal(nil), input.Signals...)
	memory.mu.RLock()
	for _, event := range memory.events {
		if event.kind != "signal" || event.occurredAt.After(at) || (event.expiresAt != nil && !at.Before(*event.expiresAt)) {
			continue
		}
		if subjectContextContains(input.Subject, event.subject) {
			signals = append(signals, ObservedSignal{Signal: event.signal, AssertedAt: event.occurredAt})
		}
	}
	events := append([]memoryEvidence(nil), memory.events...)
	memory.mu.RUnlock()
	for _, rule := range purpose.SignalRules {
		if !observesSignal(signals, rule.Signal, at) {
			continue
		}
		if rule.Effect == SignalDeny {
			decision.Reasons = []ReasonCode{"privacy_signal"}
			return decision, nil
		}
		decision.Kind = DecisionRestrict
		decision.Restrictions = append([]RestrictionKey(nil), rule.Restrictions...)
		decision.Reasons = []ReasonCode{"privacy_signal"}
		return decision, nil
	}
	if purpose.Basis != BasisConsent {
		decision.Kind = DecisionAllow
		decision.Reasons = []ReasonCode{"declared_non_consent_basis"}
		return decision, nil
	}
	latestKind := ""
	var latest memoryEvidence
	for _, event := range events {
		if event.kind == "signal" || !slices.Contains(event.purposes, ref) || !subjectContextContains(input.Subject, event.subject) ||
			event.occurredAt.After(at) {
			continue
		}
		if latestKind == "" || event.occurredAt.After(latest.occurredAt) ||
			(event.occurredAt.Equal(latest.occurredAt) && event.recordedAt.After(latest.recordedAt)) {
			latestKind, latest = event.kind, event
		}
	}
	switch latestKind {
	case "consent":
		decision.Kind = DecisionAllow
		decision.Reasons = []ReasonCode{"affirmative_consent"}
		decision.Evidence = []ReceiptID{latest.id}
	case "withdrawal":
		decision.Reasons = []ReasonCode{"consent_withdrawn"}
	default:
		decision.Reasons = []ReasonCode{"consent_missing"}
	}
	return decision, nil
}

func (memory *Memory) Consent(ctx context.Context, command ConsentCommand) (ConsentReceipt, error) {
	if err := ctx.Err(); err != nil {
		return ConsentReceipt{}, err
	}
	now := memory.clock().UTC()
	command, err := memory.prepareConsent(command, now)
	if err != nil {
		return ConsentReceipt{}, err
	}
	fingerprint := fingerprint(command)
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if existing, exists := memory.commands[command.IdempotencyKey]; exists {
		if existing.fingerprint != fingerprint || existing.kind != "consent" {
			return ConsentReceipt{}, conflict("idempotency_key", "is reused with a different command")
		}
		value := existing.consent
		value.Replay = true
		return value, nil
	}
	receipt := ConsentReceipt{
		ID: receiptID("consent", fingerprint), Subject: command.Subject, Notice: command.Notice,
		Purposes: append([]PurposeRef(nil), command.Purposes...), OccurredAt: command.OccurredAt,
		RecordedAt: now, Channel: command.Channel, EvidenceDigest: command.EvidenceDigest,
		Fingerprint: fingerprint,
	}
	memory.commands[command.IdempotencyKey] = memoryCommand{fingerprint: fingerprint, kind: "consent", consent: receipt}
	memory.events = append(memory.events, memoryEvidence{
		kind: "consent", subject: command.Subject, purposes: append([]PurposeRef(nil), command.Purposes...),
		occurredAt: command.OccurredAt, recordedAt: now, id: receipt.ID,
	})
	return receipt, nil
}

func (memory *Memory) Withdraw(ctx context.Context, command WithdrawalCommand) (WithdrawalReceipt, error) {
	if err := ctx.Err(); err != nil {
		return WithdrawalReceipt{}, err
	}
	now := memory.clock().UTC()
	command, err := memory.prepareWithdrawal(command, now)
	if err != nil {
		return WithdrawalReceipt{}, err
	}
	fingerprint := fingerprint(command)
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if existing, exists := memory.commands[command.IdempotencyKey]; exists {
		if existing.fingerprint != fingerprint || existing.kind != "withdrawal" {
			return WithdrawalReceipt{}, conflict("idempotency_key", "is reused with a different command")
		}
		value := existing.withdrawal
		value.Replay = true
		return value, nil
	}
	var supersedes []ReceiptID
	for _, event := range memory.events {
		if event.kind == "consent" && event.subject == command.Subject && event.occurredAt.Before(command.OccurredAt) {
			for _, purpose := range command.Purposes {
				if slices.Contains(event.purposes, purpose) {
					supersedes = append(supersedes, event.id)
					break
				}
			}
		}
	}
	slices.Sort(supersedes)
	receipt := WithdrawalReceipt{
		ID: receiptID("withdrawal", fingerprint), Subject: command.Subject,
		Purposes: append([]PurposeRef(nil), command.Purposes...), Supersedes: supersedes,
		OccurredAt: command.OccurredAt, RecordedAt: now, Channel: command.Channel,
		Reason: command.Reason, Fingerprint: fingerprint,
	}
	memory.commands[command.IdempotencyKey] = memoryCommand{fingerprint: fingerprint, kind: "withdrawal", withdrawal: receipt}
	memory.events = append(memory.events, memoryEvidence{
		kind: "withdrawal", subject: command.Subject, purposes: append([]PurposeRef(nil), command.Purposes...),
		occurredAt: command.OccurredAt, recordedAt: now, id: receipt.ID,
	})
	return receipt, nil
}

func (memory *Memory) ObserveSignal(ctx context.Context, command SignalCommand) (SignalReceipt, error) {
	if err := ctx.Err(); err != nil {
		return SignalReceipt{}, err
	}
	now := memory.clock().UTC()
	command, err := memory.prepareSignal(command, now)
	if err != nil {
		return SignalReceipt{}, err
	}
	fingerprint := fingerprint(command)
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if existing, exists := memory.commands[command.IdempotencyKey]; exists {
		if existing.fingerprint != fingerprint || existing.kind != "signal" {
			return SignalReceipt{}, conflict("idempotency_key", "is reused with a different command")
		}
		value := existing.signal
		value.Replay = true
		return value, nil
	}
	receipt := SignalReceipt{
		ID: receiptID("signal", fingerprint), Subject: command.Subject, Signal: command.Signal,
		AssertedAt: command.AssertedAt, ExpiresAt: command.ExpiresAt, RecordedAt: now,
		Channel: command.Channel, Fingerprint: fingerprint,
	}
	memory.commands[command.IdempotencyKey] = memoryCommand{fingerprint: fingerprint, kind: "signal", signal: receipt}
	memory.events = append(memory.events, memoryEvidence{
		kind: "signal", subject: command.Subject, signal: command.Signal,
		occurredAt: command.AssertedAt, recordedAt: now, expiresAt: command.ExpiresAt, id: receipt.ID,
	})
	return receipt, nil
}

func (memory *Memory) Track(ctx context.Context, command RetentionCommand) (RetentionItem, error) {
	if err := ctx.Err(); err != nil {
		return RetentionItem{}, err
	}
	now := memory.clock().UTC()
	if err := memory.validateIdempotency(command.IdempotencyKey); err != nil {
		return RetentionItem{}, err
	}
	if strings.TrimSpace(command.Record.Value) == "" || len(command.Record.Value) > memory.catalog.limits.MaxReferenceBytes ||
		!validKey(string(command.Record.Dataset)) || command.TriggeredAt.IsZero() {
		return RetentionItem{}, invalid("retention", "record and trigger are required")
	}
	rule, exists := memory.catalog.retention[command.Rule]
	if !exists {
		return RetentionItem{}, &Error{Kind: ErrorNotFound, Field: "rule", Message: "is not defined"}
	}
	command.TriggeredAt = command.TriggeredAt.UTC()
	fingerprint := fingerprint(command)
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if existing, exists := memory.retentionCommands[command.IdempotencyKey]; exists {
		if existing.fingerprint != fingerprint {
			return RetentionItem{}, conflict("idempotency_key", "is reused with a different command")
		}
		value := memory.retentionItems[existing.itemID]
		value.Replay = true
		return value, nil
	}
	item := RetentionItem{
		ID: RetentionItemID(receiptID("retention", fingerprint)), Record: command.Record, Rule: command.Rule,
		TriggeredAt: command.TriggeredAt, ReviewAt: rule.ReviewAfter.Add(command.TriggeredAt).UTC(),
		State: RetentionTracked, Fingerprint: fingerprint,
	}
	if !item.ReviewAt.After(now) {
		item.State = RetentionDue
	}
	memory.retentionItems[item.ID] = item
	memory.retentionCommands[command.IdempotencyKey] = memoryRetentionCommand{fingerprint: fingerprint, itemID: item.ID}
	return item, nil
}

func (memory *Memory) Review(ctx context.Context, command RetentionReviewCommand) (RetentionItem, error) {
	if err := ctx.Err(); err != nil {
		return RetentionItem{}, err
	}
	if err := memory.validateIdempotency(command.IdempotencyKey); err != nil {
		return RetentionItem{}, err
	}
	if command.ItemID == "" {
		return RetentionItem{}, invalid("item_id", "is required")
	}
	if !validDisposition(command.Outcome) {
		return RetentionItem{}, invalid("outcome", "is invalid")
	}
	if (command.Outcome == DispositionRetained || command.Outcome == DispositionRefused) && command.Reason == "" {
		return RetentionItem{}, invalid("reason", "is required for retained or refused outcomes")
	}
	if command.Outcome == DispositionRetained && command.ReviewAfter == nil && strings.TrimSpace(command.HoldRef) == "" {
		return RetentionItem{}, invalid("review_after", "is required for retained outcomes without a hold")
	}
	fingerprint := fingerprint(command)
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if existing, exists := memory.retentionCommands[command.IdempotencyKey]; exists {
		if existing.fingerprint != fingerprint {
			return RetentionItem{}, conflict("idempotency_key", "is reused with a different command")
		}
		value, exists := memory.retentionItems[existing.itemID]
		if !exists {
			return RetentionItem{}, &Error{Kind: ErrorNotFound, Field: "item_id", Message: "is not found"}
		}
		value.Replay = true
		return value, nil
	}
	item, exists := memory.retentionItems[command.ItemID]
	if !exists {
		return RetentionItem{}, &Error{Kind: ErrorNotFound, Field: "item_id", Message: "is not found"}
	}
	item.LastOutcome, item.Reason = command.Outcome, command.Reason
	if command.Outcome == DispositionRetained {
		item.State = RetentionRetained
		if command.ReviewAfter != nil {
			item.ReviewAt = command.ReviewAfter.UTC()
		}
	} else {
		item.State = RetentionCompleted
	}
	memory.retentionItems[item.ID] = item
	memory.retentionCommands[command.IdempotencyKey] = memoryRetentionCommand{fingerprint: fingerprint, itemID: item.ID}
	return item, nil
}

func (memory *Memory) Due(ctx context.Context, query RetentionDueQuery) (RetentionPage, error) {
	if err := ctx.Err(); err != nil {
		return RetentionPage{}, err
	}
	at := query.At
	if at.IsZero() {
		at = memory.clock()
	}
	limit := query.Limit
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > memory.catalog.limits.MaxDuePage {
		return RetentionPage{}, invalid("limit", "is out of range")
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	var values []RetentionItem
	for id, item := range memory.retentionItems {
		if item.State == RetentionCompleted || item.ReviewAt.After(at) || (query.Cursor != "" && string(id) <= query.Cursor) {
			continue
		}
		if item.State == RetentionTracked || item.State == RetentionRetained {
			item.State = RetentionDue
			memory.retentionItems[id] = item
		}
		values = append(values, item)
	}
	slices.SortFunc(values, func(a, b RetentionItem) int { return strings.Compare(string(a.ID), string(b.ID)) })
	page := RetentionPage{}
	if len(values) > limit {
		page.Items = append([]RetentionItem(nil), values[:limit]...)
		page.NextCursor = string(values[limit-1].ID)
	} else {
		page.Items = values
	}
	return page, nil
}

func (memory *Memory) prepareConsent(command ConsentCommand, now time.Time) (ConsentCommand, error) {
	if err := memory.validateIdempotency(command.IdempotencyKey); err != nil {
		return command, err
	}
	if err := memory.validateSubject(command.Subject); err != nil {
		return command, err
	}
	notice, exists := memory.catalog.notices[command.Notice]
	if !exists {
		return command, &Error{Kind: ErrorNotFound, Field: "notice", Message: "is not defined"}
	}
	if len(command.Purposes) == 0 || len(command.Purposes) > memory.catalog.limits.MaxPurposesPerReceipt {
		return command, invalid("purposes", "has an invalid size")
	}
	for _, purposeRef := range command.Purposes {
		purpose, exists := memory.catalog.purposes[purposeRef]
		if !exists || purpose.Basis != BasisConsent || !slices.Contains(notice.Purposes, purposeRef) {
			return command, invalid("purposes", "must be exact consent purposes included by the notice")
		}
	}
	if command.OccurredAt.IsZero() {
		command.OccurredAt = now
	}
	command.OccurredAt = command.OccurredAt.UTC()
	if command.OccurredAt.After(now.Add(5 * time.Minute)) {
		return command, invalid("occurred_at", "is too far in the future")
	}
	if command.Channel == "" {
		return command, invalid("channel", "is required")
	}
	command.Purposes = sortedUniquePurposes(command.Purposes)
	return command, nil
}

func (memory *Memory) prepareWithdrawal(command WithdrawalCommand, now time.Time) (WithdrawalCommand, error) {
	if err := memory.validateIdempotency(command.IdempotencyKey); err != nil {
		return command, err
	}
	if err := memory.validateSubject(command.Subject); err != nil {
		return command, err
	}
	if len(command.Purposes) == 0 || len(command.Purposes) > memory.catalog.limits.MaxPurposesPerReceipt {
		return command, invalid("purposes", "has an invalid size")
	}
	for _, purposeRef := range command.Purposes {
		purpose, exists := memory.catalog.purposes[purposeRef]
		if !exists || purpose.Basis != BasisConsent {
			return command, invalid("purposes", "must reference exact consent purposes")
		}
	}
	if command.OccurredAt.IsZero() {
		command.OccurredAt = now
	}
	command.OccurredAt = command.OccurredAt.UTC()
	if command.Channel == "" {
		return command, invalid("channel", "is required")
	}
	command.Purposes = sortedUniquePurposes(command.Purposes)
	return command, nil
}

func (memory *Memory) prepareSignal(command SignalCommand, now time.Time) (SignalCommand, error) {
	if err := memory.validateIdempotency(command.IdempotencyKey); err != nil {
		return command, err
	}
	if err := memory.validateSubject(command.Subject); err != nil {
		return command, err
	}
	definition, exists := memory.catalog.signals[command.Signal]
	if !exists {
		return command, &Error{Kind: ErrorNotFound, Field: "signal", Message: "is not defined"}
	}
	if command.AssertedAt.IsZero() {
		command.AssertedAt = now
	}
	command.AssertedAt = command.AssertedAt.UTC()
	if command.ExpiresAt == nil && definition.MaxEvidenceAge > 0 {
		value := command.AssertedAt.Add(definition.MaxEvidenceAge)
		command.ExpiresAt = &value
	}
	if command.ExpiresAt != nil && !command.ExpiresAt.After(command.AssertedAt) {
		return command, invalid("expires_at", "must be after asserted_at")
	}
	if command.Channel == "" {
		return command, invalid("channel", "is required")
	}
	return command, nil
}

func (memory *Memory) validateIdempotency(key IdempotencyKey) error {
	value := strings.TrimSpace(string(key))
	if value == "" || len(value) > memory.catalog.limits.MaxIdempotencyBytes || strings.ContainsRune(value, '\x00') {
		return invalid("idempotency_key", "is invalid")
	}
	return nil
}

func (memory *Memory) validateSubject(subject SubjectRef) error {
	definition, exists := memory.catalog.subjectKinds[subject.Kind]
	if !exists || !validKey(string(subject.Owner)) || strings.TrimSpace(subject.Value) == "" ||
		len(subject.Value) > definition.MaxRefBytes || strings.ContainsRune(subject.Value, '\x00') {
		return invalid("subject", "is invalid")
	}
	return nil
}

func sortedUniquePurposes(values []PurposeRef) []PurposeRef {
	result := append([]PurposeRef(nil), values...)
	slices.SortFunc(result, func(a, b PurposeRef) int { return compareRefs(string(a.Key), a.Revision, string(b.Key), b.Revision) })
	return slices.Compact(result)
}

func observesSignal(values []ObservedSignal, key SignalKey, at time.Time) bool {
	for _, value := range values {
		if value.Signal == key && !value.AssertedAt.After(at) {
			return true
		}
	}
	return false
}

func subjectContextContains(context SubjectContext, subject SubjectRef) bool {
	if context.Current != nil && *context.Current == subject {
		return true
	}
	return slices.Contains(context.Aliases, subject)
}

func validDisposition(value OwnerDisposition) bool {
	switch value {
	case DispositionExported, DispositionRectified, DispositionRestricted, DispositionDeleted,
		DispositionAnonymized, DispositionRetained, DispositionNotFound, DispositionRefused:
		return true
	default:
		return false
	}
}

func fingerprint(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func receiptID(prefix, value string) ReceiptID {
	return ReceiptID(prefix + "_" + value[:24])
}

func cursorIndex(value string) int {
	index, _ := strconv.Atoi(value)
	return index
}
