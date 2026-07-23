package abuse

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"slices"
	"sync"
	"time"
)

type MemoryOptions struct {
	Clock      func() time.Time
	Secret     []byte
	KeyVersion uint32
	Verifiers  map[ChallengeKind]ChallengeVerifier
}

type memoryStore struct {
	mu       sync.Mutex
	catalog  *Catalog
	clock    func() time.Time
	states   map[stateKey]meterState
	receipts map[AttemptID]receiptRecord
	policies map[PolicyID]compiledMeter
}

func NewMemory(catalog *Catalog, options MemoryOptions) (Module, error) {
	if catalog == nil {
		return nil, invalidInput("catalog", "is required")
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	secret, err := normalizeSecret(options.Secret)
	if err != nil {
		return nil, err
	}
	keyVersion := options.KeyVersion
	if keyVersion == 0 {
		keyVersion = 1
	}
	store := &memoryStore{
		catalog: catalog, clock: clock, states: make(map[stateKey]meterState),
		receipts: make(map[AttemptID]receiptRecord), policies: make(map[PolicyID]compiledMeter),
	}
	for _, action := range catalog.actions {
		for _, meter := range action.meters {
			store.policies[meter.def.ID] = meter
		}
	}
	return &runtime{
		catalog: catalog, store: store, secret: secret, keyVersion: keyVersion,
		verifiers: cloneVerifiers(options.Verifiers),
	}, nil
}

func normalizeSecret(value []byte) ([32]byte, error) {
	var result [32]byte
	switch {
	case len(value) == 0:
		if _, err := rand.Read(result[:]); err != nil {
			return result, &Error{Kind: ErrorStoreUnavailable, Field: "secret", Message: "cannot generate subject secret", Cause: err}
		}
	case len(value) < 32:
		return result, invalidInput("secret", "must contain at least 32 bytes")
	default:
		result = sha256.Sum256(value)
	}
	return result, nil
}

func cloneVerifiers(value map[ChallengeKind]ChallengeVerifier) map[ChallengeKind]ChallengeVerifier {
	result := make(map[ChallengeKind]ChallengeVerifier, len(value))
	for key, verifier := range value {
		result[key] = verifier
	}
	return result
}

func (store *memoryStore) admit(ctx context.Context, attempt preparedAttempt, proofSatisfied bool) (Admission, error) {
	if err := ctx.Err(); err != nil {
		return Admission{}, err
	}
	now := store.clock().UTC()
	store.mu.Lock()
	defer store.mu.Unlock()
	if existing, ok := store.receipts[attempt.id]; ok {
		if existing.Fingerprint != attempt.fingerprint || existing.Action != attempt.action.def.Key {
			return Admission{}, conflict("id", "is reused with a different attempt")
		}
		if existing.Disposition != DispositionChallenge || !proofSatisfied {
			return admissionFromRecord(existing, true), nil
		}
	}

	states := make(map[stateKey]meterState, len(attempt.meters))
	findings := make([]Finding, 0, len(attempt.meters))
	disposition := DispositionAllow
	var retry time.Time
	for _, prepared := range attempt.meters {
		state, exists := store.states[prepared.key]
		if !exists {
			state = newMeterState(prepared.compiled, now)
		}
		settleState(&state, prepared.compiled, now)
		used := stateUsage(state, prepared.compiled)
		candidate := used + prepared.compiled.def.Cost
		finding := Finding{
			Policy: prepared.compiled.def.ID, Used: candidate,
			Capacity: prepared.compiled.def.Algorithm.Capacity,
		}
		switch {
		case candidate > prepared.compiled.def.Algorithm.Capacity:
			finding.Disposition = DispositionReject
			finding.RetryAt = retryAt(state, prepared.compiled, now, prepared.compiled.def.Cost)
			disposition = DispositionReject
			if finding.RetryAt.After(retry) {
				retry = finding.RetryAt
			}
		case prepared.compiled.def.ChallengeAt > 0 &&
			candidate >= prepared.compiled.def.ChallengeAt && !proofSatisfied:
			finding.Disposition = DispositionChallenge
			if disposition != DispositionReject {
				disposition = DispositionChallenge
			}
		default:
			finding.Disposition = DispositionAllow
		}
		states[prepared.key] = state
		findings = append(findings, finding)
	}

	record := receiptRecord{
		ID: attempt.id, Action: attempt.action.def.Key, Fingerprint: attempt.fingerprint,
		Disposition: disposition, RetryAt: retry, Findings: findings,
		CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(store.catalog.limits.ReceiptRetention),
	}
	if previous, ok := store.receipts[attempt.id]; ok {
		record.CreatedAt = previous.CreatedAt
	}
	if disposition == DispositionChallenge {
		record.Challenge = &Challenge{
			Kind: attempt.action.def.Challenge.Kind, ContinuationID: attempt.id,
		}
		for key, state := range states {
			store.states[key] = state
		}
		store.receipts[attempt.id] = record
		return admissionFromRecord(record, false), nil
	}
	if disposition == DispositionReject {
		for key, state := range states {
			store.states[key] = state
		}
		store.receipts[attempt.id] = record
		return admissionFromRecord(record, false), nil
	}

	var pendingTTL time.Duration
	if attempt.action.def.Resolution != nil {
		pendingTTL = attempt.action.def.Resolution.PendingTTL
		record.DefaultOutcome = attempt.action.def.Resolution.DefaultOutcome
		record.PendingUntil = now.Add(pendingTTL)
		for outcome := range attempt.action.outcomes {
			record.AllowedOutcomes = append(record.AllowedOutcomes, outcome)
		}
		slices.Sort(record.AllowedOutcomes)
		if expiration := record.PendingUntil.Add(store.catalog.limits.ReceiptRetention); expiration.After(record.ExpiresAt) {
			record.ExpiresAt = expiration
		}
	}
	for _, prepared := range attempt.meters {
		state := states[prepared.key]
		consumeState(&state, prepared.compiled, attempt.id, now, record.DefaultOutcome, pendingTTL)
		store.states[prepared.key] = state
		if prepared.compiled.def.Mode == MeterOutcome {
			record.ResolutionRefs = append(record.ResolutionRefs, resolutionRef{Key: prepared.key})
		}
	}
	store.receipts[attempt.id] = record
	return admissionFromRecord(record, false), nil
}

func admissionFromRecord(record receiptRecord, replay bool) Admission {
	result := Admission{
		Disposition: record.Disposition, RetryAt: record.RetryAt, Replay: replay,
		Findings: append([]Finding(nil), record.Findings...),
	}
	if record.Challenge != nil {
		value := *record.Challenge
		result.Challenge = &value
	}
	if record.Disposition == DispositionAllow {
		result.Receipt = Receipt{
			id: record.ID, action: record.Action, fingerprint: record.Fingerprint,
		}
	}
	return result
}

func (store *memoryStore) resolve(ctx context.Context, receipt Receipt, outcome OutcomeKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.resolveLocked(receipt, outcome, store.clock().UTC())
}

func (store *memoryStore) resolveLocked(receipt Receipt, outcome OutcomeKey, now time.Time) error {
	record, ok := store.receipts[receipt.id]
	if !ok || record.Fingerprint != receipt.fingerprint || record.Action != receipt.action {
		return conflict("receipt", "does not match an admitted attempt")
	}
	if record.Disposition != DispositionAllow || len(record.AllowedOutcomes) == 0 {
		return conflict("receipt", "does not require resolution")
	}
	if record.ResolvedOutcome != "" {
		if record.ResolvedOutcome == outcome {
			return nil
		}
		return conflict("outcome", "attempt was already resolved differently")
	}
	if !slices.Contains(record.AllowedOutcomes, outcome) {
		return invalidInput("outcome", "is not allowed by the receipt")
	}
	for _, ref := range record.ResolutionRefs {
		state, exists := store.states[ref.Key]
		if !exists {
			continue
		}
		meter, exists := store.policies[ref.Key.policy]
		if !exists {
			return &Error{Kind: ErrorDefinitionDrift, Field: "policy", Message: "receipt policy is unavailable"}
		}
		resolveState(&state, meter, record.ID, outcome, now)
		store.states[ref.Key] = state
	}
	record.ResolvedOutcome = outcome
	record.UpdatedAt = now
	store.receipts[receipt.id] = record
	return nil
}

func (store *memoryStore) inspect(ctx context.Context, attempt preparedAttempt) (Inspection, error) {
	if err := ctx.Err(); err != nil {
		return Inspection{}, err
	}
	now := store.clock().UTC()
	store.mu.Lock()
	defer store.mu.Unlock()
	result := Inspection{Action: attempt.action.def.Key}
	for _, prepared := range attempt.meters {
		state, exists := store.states[prepared.key]
		if !exists {
			state = newMeterState(prepared.compiled, now)
		}
		settleState(&state, prepared.compiled, now)
		store.states[prepared.key] = state
		result.Meters = append(result.Meters, MeterSnapshot{
			Policy: prepared.compiled.def.ID, Slot: prepared.slot,
			SubjectRef: prepared.key.subject.hex(), Used: stateUsage(state, prepared.compiled),
			Capacity: prepared.compiled.def.Algorithm.Capacity, ExpiresAt: state.ExpiresAt,
		})
	}
	return result, nil
}

func (store *memoryStore) reset(ctx context.Context, attempt preparedAttempt) (ResetResult, error) {
	if err := ctx.Err(); err != nil {
		return ResetResult{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	var result ResetResult
	for _, prepared := range attempt.meters {
		if state, exists := store.states[prepared.key]; exists {
			result.MetersReset++
			result.EventsRemoved += int64(len(state.Events))
			delete(store.states, prepared.key)
		}
	}
	return result, nil
}

func (store *memoryStore) prune(ctx context.Context, command PruneCommand) (PruneResult, error) {
	if err := ctx.Err(); err != nil {
		return PruneResult{}, err
	}
	now := store.clock().UTC()
	store.mu.Lock()
	defer store.mu.Unlock()
	var result PruneResult
	processed := 0
	for key, state := range store.states {
		if processed >= command.Limit {
			break
		}
		meter, ok := store.policies[key.policy]
		if !ok {
			if state.ExpiresAt.Before(command.Before) {
				result.EventsRemoved += int64(len(state.Events))
				delete(store.states, key)
				result.StatesRemoved++
				processed++
			}
			continue
		}
		removed, finalized := settleState(&state, meter, now)
		result.EventsRemoved += removed
		result.PendingFinalized += finalized
		if state.ExpiresAt.Before(command.Before) && stateUsage(state, meter) == 0 {
			delete(store.states, key)
			result.StatesRemoved++
			processed++
		} else {
			store.states[key] = state
		}
	}
	for id, record := range store.receipts {
		if processed >= command.Limit {
			break
		}
		if record.ResolvedOutcome == "" && record.Disposition == DispositionAllow &&
			record.DefaultOutcome != "" && !now.Before(record.PendingUntil) {
			receipt := Receipt{id: id, action: record.Action, fingerprint: record.Fingerprint}
			if err := store.resolveLocked(receipt, record.DefaultOutcome, now); err != nil {
				return result, err
			}
			result.PendingFinalized++
		}
		record = store.receipts[id]
		if record.ExpiresAt.Before(command.Before) &&
			(record.Disposition != DispositionAllow || record.ResolvedOutcome != "" || record.DefaultOutcome == "") {
			delete(store.receipts, id)
			result.ReceiptsRemoved++
			processed++
		}
	}
	return result, nil
}
