package abuse

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"
)

type stateKey struct {
	policy     PolicyID
	keyVersion uint32
	subject    subjectKey
}

func (key stateKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Policy     PolicyID `json:"policy"`
		KeyVersion uint32   `json:"keyVersion"`
		Subject    string   `json:"subject"`
	}{key.policy, key.keyVersion, key.subject.hex()})
}

func (key *stateKey) UnmarshalJSON(value []byte) error {
	var encoded struct {
		Policy     PolicyID `json:"policy"`
		KeyVersion uint32   `json:"keyVersion"`
		Subject    string   `json:"subject"`
	}
	if err := json.Unmarshal(value, &encoded); err != nil {
		return err
	}
	decoded, err := hex.DecodeString(encoded.Subject)
	if err != nil || len(decoded) != 32 {
		return fmt.Errorf("invalid subject key")
	}
	key.policy = encoded.Policy
	key.keyVersion = encoded.KeyVersion
	copy(key.subject[:], decoded)
	return nil
}

func (key stateKey) less(other stateKey) int {
	if value := strings.Compare(string(key.policy), string(other.policy)); value != 0 {
		return value
	}
	if key.keyVersion < other.keyVersion {
		return -1
	}
	if key.keyVersion > other.keyVersion {
		return 1
	}
	return bytes.Compare(key.subject[:], other.subject[:])
}

type meterEvent struct {
	AttemptID     AttemptID `json:"attemptId"`
	Cost          int64     `json:"cost"`
	OccurredAt    time.Time `json:"occurredAt"`
	Pending       bool      `json:"pending,omitempty"`
	PendingUntil  time.Time `json:"pendingUntil,omitempty"`
	DefaultCharge bool      `json:"defaultCharge,omitempty"`
}

type meterState struct {
	PolicyRevision uint64        `json:"policyRevision"`
	Algorithm      AlgorithmKind `json:"algorithm"`
	Capacity       int64         `json:"capacity"`
	Available      int64         `json:"available,omitempty"`
	Remainder      int64         `json:"remainder,omitempty"`
	LastRefillAt   time.Time     `json:"lastRefillAt,omitempty"`
	FixedStartedAt time.Time     `json:"fixedStartedAt,omitempty"`
	FixedUsed      int64         `json:"fixedUsed,omitempty"`
	Events         []meterEvent  `json:"events,omitempty"`
	ExpiresAt      time.Time     `json:"expiresAt"`
}

type preparedMeter struct {
	compiled compiledMeter
	key      stateKey
	slot     SlotKey
}

type preparedAttempt struct {
	id          AttemptID
	action      *compiledAction
	meters      []preparedMeter
	fingerprint [32]byte
}

type resolutionRef struct {
	Key stateKey
}

type receiptRecord struct {
	ID              AttemptID
	Action          ActionKey
	Fingerprint     [32]byte
	Disposition     Disposition
	RetryAt         time.Time
	Challenge       *Challenge
	Findings        []Finding
	ResolutionRefs  []resolutionRef
	AllowedOutcomes []OutcomeKey
	DefaultOutcome  OutcomeKey
	ResolvedOutcome OutcomeKey
	PendingUntil    time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ExpiresAt       time.Time
}

type store interface {
	admit(context.Context, preparedAttempt, bool) (Admission, error)
	resolve(context.Context, Receipt, OutcomeKey) error
	inspect(context.Context, preparedAttempt) (Inspection, error)
	reset(context.Context, preparedAttempt) (ResetResult, error)
	prune(context.Context, PruneCommand) (PruneResult, error)
}

type runtime struct {
	catalog    *Catalog
	store      store
	secret     [32]byte
	keyVersion uint32
	verifiers  map[ChallengeKind]ChallengeVerifier
}

type boundAction struct {
	runtime *runtime
	action  *compiledAction
}

var _ Module = (*runtime)(nil)
var _ Action = (*boundAction)(nil)

func (module *runtime) Action(key ActionKey) (Action, error) {
	if module == nil || module.catalog == nil {
		return nil, &Error{Kind: ErrorStoreUnavailable, Field: "module", Message: "is not initialized"}
	}
	key = ActionKey(strings.TrimSpace(string(key)))
	action, ok := module.catalog.actions[key]
	if !ok {
		return nil, &Error{Kind: ErrorUnknownAction, Field: "action", Message: "is not registered"}
	}
	return &boundAction{runtime: module, action: action}, nil
}

func (module *runtime) Governor() Governor {
	return &governor{runtime: module}
}

func (action *boundAction) Key() ActionKey {
	if action == nil || action.action == nil {
		return ""
	}
	return action.action.def.Key
}

func (action *boundAction) Admit(ctx context.Context, input Input) (Admission, error) {
	if action == nil || action.runtime == nil || action.action == nil {
		return Admission{}, &Error{Kind: ErrorStoreUnavailable, Field: "action", Message: "is not initialized"}
	}
	if err := ctx.Err(); err != nil {
		return Admission{}, err
	}
	prepared, err := action.runtime.prepare(action.action, input)
	if err != nil {
		return Admission{}, err
	}
	admission, err := action.runtime.store.admit(ctx, prepared, false)
	if err != nil || admission.Disposition != DispositionChallenge || input.Proof == nil {
		return admission, err
	}
	challenge := action.action.def.Challenge
	if challenge == nil {
		return Admission{}, &Error{Kind: ErrorVerifierConfiguration, Field: "challenge", Message: "action has no challenge definition"}
	}
	if input.Proof.Kind != challenge.Kind {
		return admission, nil
	}
	if len(input.Proof.Token) == 0 || len(input.Proof.Token) > action.runtime.catalog.limits.MaxProofBytes {
		return admission, nil
	}
	verifier := action.runtime.verifiers[challenge.Kind]
	if verifier == nil {
		return Admission{}, &Error{Kind: ErrorVerifierConfiguration, Field: "challenge", Message: "verifier is not configured"}
	}
	verification, err := verifier.Verify(ctx, VerificationRequest{
		VerificationID: verificationID(prepared.id, prepared.fingerprint),
		Token:          input.Proof.Token, ExpectedAction: challenge.ExpectedAction,
		AllowedHosts: append([]string(nil), challenge.AllowedHosts...),
	})
	if err != nil {
		var typed *Error
		if errorsAs(err, &typed) {
			return Admission{}, err
		}
		return Admission{}, &Error{
			Kind: ErrorVerifierUnavailable, Field: "challenge",
			Message: "verification failed", Retryable: true, Cause: err,
		}
	}
	if verification.Status != VerificationAccepted {
		return admission, nil
	}
	if verification.Action != challenge.ExpectedAction ||
		!slices.Contains(challenge.AllowedHosts, verification.Hostname) {
		return admission, nil
	}
	return action.runtime.store.admit(ctx, prepared, true)
}

func (action *boundAction) Resolve(ctx context.Context, receipt Receipt, outcome OutcomeKey) error {
	if action == nil || action.runtime == nil || action.action == nil {
		return &Error{Kind: ErrorStoreUnavailable, Field: "action", Message: "is not initialized"}
	}
	if receipt.IsZero() || receipt.action != action.action.def.Key {
		return invalidInput("receipt", "does not belong to this action")
	}
	outcome = OutcomeKey(strings.TrimSpace(string(outcome)))
	if _, ok := action.action.outcomes[outcome]; !ok {
		return invalidInput("outcome", "is not declared by the action")
	}
	return action.runtime.store.resolve(ctx, receipt, outcome)
}

func (module *runtime) prepare(action *compiledAction, input Input) (preparedAttempt, error) {
	id := AttemptID(strings.TrimSpace(string(input.ID)))
	if id == "" {
		return preparedAttempt{}, invalidInput("id", "is required")
	}
	if len(id) > module.catalog.limits.MaxAttemptIDBytes {
		return preparedAttempt{}, invalidInput("id", "exceeds maximum size")
	}
	signals, err := module.normalizeSignals(action, input.Signals)
	if err != nil {
		return preparedAttempt{}, err
	}
	prepared := preparedAttempt{id: id, action: action}
	for _, meter := range action.meters {
		value, present := signals[meter.def.Slot]
		if !present {
			continue
		}
		key := module.deriveSubject(meter.def.ID, meter.def.Slot, value)
		prepared.meters = append(prepared.meters, preparedMeter{
			compiled: meter, key: stateKey{
				policy: meter.def.ID, keyVersion: module.keyVersion, subject: key,
			}, slot: meter.def.Slot,
		})
	}
	if len(prepared.meters) == 0 {
		return preparedAttempt{}, invalidInput("signals", "do not activate any meter")
	}
	slices.SortFunc(prepared.meters, func(a, b preparedMeter) int { return a.key.less(b.key) })
	hash := sha256.New()
	_, _ = hash.Write([]byte("foundation.abuse.attempt.v1"))
	writeString(hash, string(action.def.Key))
	for _, meter := range prepared.meters {
		writeString(hash, string(meter.key.policy))
		writeString(hash, meter.key.subject.hex())
	}
	copy(prepared.fingerprint[:], hash.Sum(nil))
	return prepared, nil
}

func (module *runtime) normalizeSignals(action *compiledAction, value Signals) (map[SlotKey]string, error) {
	result := make(map[SlotKey]string, 3+len(value.Extra))
	if value.Network.IsValid() {
		result[SlotNetwork] = value.Network.Masked().String()
	}
	if actor := strings.TrimSpace(value.Actor); actor != "" {
		result[SlotActor] = actor
	}
	if target := strings.TrimSpace(value.Target); target != "" {
		result[SlotTarget] = target
	}
	if len(value.Extra) > module.catalog.limits.MaxExtraSignals {
		return nil, invalidInput("signals.extra", "exceeds maximum items")
	}
	for _, signal := range value.Extra {
		slot := SlotKey(strings.TrimSpace(string(signal.Slot)))
		canonical := strings.TrimSpace(signal.Canonical)
		if _, allowed := action.allowedSlots[slot]; !allowed || slot == SlotNetwork || slot == SlotActor || slot == SlotTarget {
			return nil, invalidInput("signals.extra", "contains an undeclared slot")
		}
		if canonical == "" {
			return nil, invalidInput("signals.extra", "contains an empty value")
		}
		if _, duplicate := result[slot]; duplicate {
			return nil, invalidInput("signals.extra", "contains a duplicate slot")
		}
		result[slot] = canonical
	}
	for _, slot := range action.requiredSlots {
		if result[slot] == "" {
			return nil, invalidInput("signals", "is missing a required slot")
		}
	}
	for _, canonical := range result {
		if len(canonical) > module.catalog.limits.MaxSignalBytes || strings.ContainsRune(canonical, '\x00') {
			return nil, invalidInput("signals", "contains an invalid value")
		}
	}
	return result, nil
}

func (module *runtime) deriveSubject(policy PolicyID, slot SlotKey, canonical string) subjectKey {
	mac := hmac.New(sha256.New, module.secret[:])
	_, _ = mac.Write([]byte("foundation.abuse.subject.v1"))
	writeString(mac, module.catalog.consumer)
	var version [4]byte
	binary.BigEndian.PutUint32(version[:], module.keyVersion)
	_, _ = mac.Write(version[:])
	writeString(mac, string(policy))
	writeString(mac, string(slot))
	writeString(mac, canonical)
	var result subjectKey
	copy(result[:], mac.Sum(nil))
	return result
}

type stringWriter interface {
	Write([]byte) (int, error)
}

func writeString(writer stringWriter, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = writer.Write(size[:])
	_, _ = writer.Write([]byte(value))
}

func verificationID(id AttemptID, fingerprint [32]byte) string {
	sum := sha256.Sum256(append(append([]byte("foundation.abuse.verification.v1\x00"), []byte(id)...), fingerprint[:]...))
	value := sum[:16]
	value[6] = (value[6] & 0x0f) | 0x50
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}

func newMeterState(meter compiledMeter, now time.Time) meterState {
	state := meterState{
		PolicyRevision: meter.def.Revision, Algorithm: meter.def.Algorithm.Kind,
		Capacity: meter.def.Algorithm.Capacity, ExpiresAt: now.Add(meter.def.Retention),
	}
	if meter.def.Algorithm.Kind == AlgorithmTokenBucket {
		state.Available = meter.def.Algorithm.Capacity
		state.LastRefillAt = now
	}
	if meter.def.Algorithm.Kind == AlgorithmFixedWindow {
		state.FixedStartedAt = fixedWindowStart(now, meter.def.Algorithm.Window)
	}
	return state
}

func settleState(state *meterState, meter compiledMeter, now time.Time) (eventsRemoved, pendingFinalized int64) {
	if state.Capacity != meter.def.Algorithm.Capacity {
		if state.Algorithm == AlgorithmTokenBucket {
			used := state.Capacity - state.Available
			state.Available = meter.def.Algorithm.Capacity - used
			if state.Available < 0 {
				state.Available = 0
			}
			if state.Available > meter.def.Algorithm.Capacity {
				state.Available = meter.def.Algorithm.Capacity
			}
		}
		state.Capacity = meter.def.Algorithm.Capacity
	}
	state.PolicyRevision = meter.def.Revision
	switch meter.def.Algorithm.Kind {
	case AlgorithmTokenBucket:
		refillTokenBucket(state, meter.def.Algorithm, now)
	case AlgorithmFixedWindow:
		start := fixedWindowStart(now, meter.def.Algorithm.Window)
		if state.FixedStartedAt.IsZero() || start.After(state.FixedStartedAt) {
			state.FixedStartedAt, state.FixedUsed = start, 0
		}
	case AlgorithmSlidingWindow:
		cutoff := now.Add(-meter.def.Algorithm.Window)
		kept := state.Events[:0]
		for _, event := range state.Events {
			if !event.OccurredAt.After(cutoff) {
				eventsRemoved++
				continue
			}
			if event.Pending && !now.Before(event.PendingUntil) {
				if event.DefaultCharge {
					event.Pending = false
					pendingFinalized++
				} else {
					eventsRemoved++
					continue
				}
			}
			kept = append(kept, event)
		}
		state.Events = kept
	}
	return eventsRemoved, pendingFinalized
}

func refillTokenBucket(state *meterState, def AlgorithmDefinition, now time.Time) {
	if state.LastRefillAt.IsZero() {
		state.LastRefillAt = now
		state.Available = def.Capacity
		return
	}
	if !now.After(state.LastRefillAt) || state.Available >= def.Capacity {
		if state.Available >= def.Capacity {
			state.Available = def.Capacity
			state.Remainder = 0
			state.LastRefillAt = now
		}
		return
	}
	elapsed := now.Sub(state.LastRefillAt).Nanoseconds()
	numerator := new(big.Int).Mul(big.NewInt(elapsed), big.NewInt(def.RefillAmount))
	numerator.Add(numerator, big.NewInt(state.Remainder))
	period := big.NewInt(def.RefillPeriod.Nanoseconds())
	added, remainder := new(big.Int), new(big.Int)
	added.QuoRem(numerator, period, remainder)
	if added.Sign() > 0 {
		if added.IsInt64() {
			state.Available += added.Int64()
		} else {
			state.Available = def.Capacity
		}
		if state.Available >= def.Capacity {
			state.Available = def.Capacity
			state.Remainder = 0
		} else {
			state.Remainder = remainder.Int64()
		}
	} else {
		state.Remainder = remainder.Int64()
	}
	state.LastRefillAt = now
}

func fixedWindowStart(now time.Time, window time.Duration) time.Time {
	nanos := now.UTC().UnixNano()
	width := window.Nanoseconds()
	return time.Unix(0, nanos-(nanos%width)).UTC()
}

func stateUsage(state meterState, meter compiledMeter) int64 {
	switch meter.def.Algorithm.Kind {
	case AlgorithmTokenBucket:
		return meter.def.Algorithm.Capacity - state.Available
	case AlgorithmFixedWindow:
		return state.FixedUsed
	case AlgorithmSlidingWindow:
		var used int64
		for _, event := range state.Events {
			used += event.Cost
		}
		return used
	default:
		return 0
	}
}

func retryAt(state meterState, meter compiledMeter, now time.Time, cost int64) time.Time {
	switch meter.def.Algorithm.Kind {
	case AlgorithmTokenBucket:
		missing := cost - state.Available
		if missing <= 0 {
			return time.Time{}
		}
		numerator := new(big.Int).Mul(big.NewInt(missing), big.NewInt(meter.def.Algorithm.RefillPeriod.Nanoseconds()))
		numerator.Sub(numerator, big.NewInt(state.Remainder))
		denominator := big.NewInt(meter.def.Algorithm.RefillAmount)
		quotient, remainder := new(big.Int), new(big.Int)
		quotient.QuoRem(numerator, denominator, remainder)
		if remainder.Sign() > 0 {
			quotient.Add(quotient, big.NewInt(1))
		}
		if !quotient.IsInt64() {
			return now.Add(meter.def.Retention)
		}
		return now.Add(time.Duration(quotient.Int64()))
	case AlgorithmFixedWindow:
		return state.FixedStartedAt.Add(meter.def.Algorithm.Window)
	case AlgorithmSlidingWindow:
		events := append([]meterEvent(nil), state.Events...)
		slices.SortFunc(events, func(a, b meterEvent) int { return a.OccurredAt.Compare(b.OccurredAt) })
		needed := stateUsage(state, meter) + cost - meter.def.Algorithm.Capacity
		var released int64
		for _, event := range events {
			released += event.Cost
			if released >= needed {
				return event.OccurredAt.Add(meter.def.Algorithm.Window)
			}
		}
	}
	return time.Time{}
}

func consumeState(state *meterState, meter compiledMeter, attempt AttemptID, now time.Time, defaultOutcome OutcomeKey, pendingTTL time.Duration) {
	state.ExpiresAt = now.Add(meter.def.Retention)
	switch meter.def.Algorithm.Kind {
	case AlgorithmTokenBucket:
		state.Available -= meter.def.Cost
	case AlgorithmFixedWindow:
		state.FixedUsed += meter.def.Cost
	case AlgorithmSlidingWindow:
		event := meterEvent{AttemptID: attempt, Cost: meter.def.Cost, OccurredAt: now}
		if meter.def.Mode == MeterOutcome {
			event.Pending = true
			event.PendingUntil = now.Add(pendingTTL)
			event.DefaultCharge = meter.chargeOn[defaultOutcome]
		}
		state.Events = append(state.Events, event)
	}
}

func resolveState(state *meterState, meter compiledMeter, attempt AttemptID, outcome OutcomeKey, now time.Time) {
	settleState(state, meter, now)
	state.ExpiresAt = now.Add(meter.def.Retention)
	kept := state.Events[:0]
	for _, event := range state.Events {
		if event.AttemptID == attempt && event.Pending {
			if meter.chargeOn[outcome] {
				event.Pending = false
				kept = append(kept, event)
			}
			continue
		}
		kept = append(kept, event)
	}
	state.Events = kept
	if meter.resetOn[outcome] {
		kept = state.Events[:0]
		for _, event := range state.Events {
			if event.Pending {
				kept = append(kept, event)
			}
		}
		state.Events = kept
	}
}
