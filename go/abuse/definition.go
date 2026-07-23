package abuse

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"time"
)

const DefinitionVersion uint64 = 1

var definitionKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,95}$`)

type Requiredness string

const (
	Optional Requiredness = "optional"
	Required Requiredness = "required"
)

type SignalRequirements struct {
	Network Requiredness
	Actor   Requiredness
	Target  Requiredness
	Extra   []SlotRequirement
}

type SlotRequirement struct {
	Slot     SlotKey
	Required Requiredness
}

type MeterMode string

const (
	MeterAdmission MeterMode = "admission"
	MeterOutcome   MeterMode = "outcome"
)

type AlgorithmKind string

const (
	AlgorithmTokenBucket   AlgorithmKind = "token_bucket"
	AlgorithmSlidingWindow AlgorithmKind = "sliding_window"
	AlgorithmFixedWindow   AlgorithmKind = "fixed_window"
)

type AlgorithmDefinition struct {
	Kind         AlgorithmKind `json:"kind"`
	Capacity     int64         `json:"capacity"`
	RefillAmount int64         `json:"refillAmount,omitempty"`
	RefillPeriod time.Duration `json:"refillPeriod,omitempty"`
	Window       time.Duration `json:"window,omitempty"`
}

func TokenBucket(capacity, refillAmount int64, refillPeriod time.Duration) AlgorithmDefinition {
	return AlgorithmDefinition{
		Kind: AlgorithmTokenBucket, Capacity: capacity,
		RefillAmount: refillAmount, RefillPeriod: refillPeriod,
	}
}

func SlidingWindow(capacity int64, window time.Duration) AlgorithmDefinition {
	return AlgorithmDefinition{Kind: AlgorithmSlidingWindow, Capacity: capacity, Window: window}
}

func FixedWindow(capacity int64, window time.Duration) AlgorithmDefinition {
	return AlgorithmDefinition{Kind: AlgorithmFixedWindow, Capacity: capacity, Window: window}
}

type MeterDefinition struct {
	ID          PolicyID
	Revision    uint64
	Slot        SlotKey
	Mode        MeterMode
	Cost        int64
	Algorithm   AlgorithmDefinition
	ChallengeAt int64
	ChargeOn    []OutcomeKey
	ResetOn     []OutcomeKey
	Retention   time.Duration
}

type ResolutionDefinition struct {
	Outcomes       []OutcomeKey
	DefaultOutcome OutcomeKey
	PendingTTL     time.Duration
}

type ChallengeDefinition struct {
	Kind           ChallengeKind
	ExpectedAction string
	AllowedHosts   []string
}

type ActionDefinition struct {
	Key        ActionKey
	Required   SignalRequirements
	Meters     []MeterDefinition
	Resolution *ResolutionDefinition
	Challenge  *ChallengeDefinition
}

type Limits struct {
	MaxAttemptIDBytes int
	MaxSignalBytes    int
	MaxExtraSignals   int
	MaxProofBytes     int
	ReceiptRetention  time.Duration
}

type Definition struct {
	Version  uint64
	Consumer string
	Actions  []ActionDefinition
	Limits   Limits
}

type Catalog struct {
	version  uint64
	consumer string
	actions  map[ActionKey]*compiledAction
	limits   Limits
	digest   string
}

type compiledAction struct {
	def           ActionDefinition
	requiredSlots []SlotKey
	allowedSlots  map[SlotKey]Requiredness
	meters        []compiledMeter
	outcomes      map[OutcomeKey]struct{}
}

type compiledMeter struct {
	def      MeterDefinition
	chargeOn map[OutcomeKey]bool
	resetOn  map[OutcomeKey]bool
}

func Compile(definition Definition) (*Catalog, error) {
	if definition.Version < DefinitionVersion {
		return nil, invalidDefinition("version", "must be at least %d", DefinitionVersion)
	}
	consumer := strings.TrimSpace(definition.Consumer)
	if !definitionKeyPattern.MatchString(consumer) {
		return nil, invalidDefinition("consumer", "must be a stable lowercase key")
	}
	limits, err := normalizeLimits(definition.Limits)
	if err != nil {
		return nil, err
	}
	if len(definition.Actions) == 0 {
		return nil, invalidDefinition("actions", "must contain at least one action")
	}
	actions := make(map[ActionKey]*compiledAction, len(definition.Actions))
	policies := map[PolicyID]struct{}{}
	canonicalActions := append([]ActionDefinition(nil), definition.Actions...)
	slices.SortFunc(canonicalActions, func(a, b ActionDefinition) int {
		return strings.Compare(string(a.Key), string(b.Key))
	})
	for index, actionDef := range canonicalActions {
		action, err := compileAction(actionDef, policies)
		if err != nil {
			return nil, prefixDefinition(err, "actions", index)
		}
		if _, exists := actions[action.def.Key]; exists {
			return nil, invalidDefinition("actions", "contains duplicate %q", action.def.Key)
		}
		actions[action.def.Key] = action
	}
	canonical := struct {
		Version  uint64
		Consumer string
		Actions  []ActionDefinition
		Limits   Limits
	}{definition.Version, consumer, canonicalActions, limits}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return nil, &Error{Kind: ErrorInvalidDefinition, Field: "definition", Message: "cannot encode canonical definition", Cause: err}
	}
	sum := sha256.Sum256(encoded)
	return &Catalog{
		version: definition.Version, consumer: consumer, actions: actions,
		limits: limits, digest: hex.EncodeToString(sum[:]),
	}, nil
}

func MustCompile(definition Definition) *Catalog {
	catalog, err := Compile(definition)
	if err != nil {
		panic(err)
	}
	return catalog
}

func compileAction(def ActionDefinition, policies map[PolicyID]struct{}) (*compiledAction, error) {
	def.Key = ActionKey(strings.TrimSpace(string(def.Key)))
	if !definitionKeyPattern.MatchString(string(def.Key)) {
		return nil, invalidDefinition("key", "must be a stable lowercase key")
	}
	required, allowed, err := compileRequirements(def.Required)
	if err != nil {
		return nil, err
	}
	if len(def.Meters) == 0 {
		return nil, invalidDefinition("meters", "must contain at least one meter")
	}
	action := &compiledAction{
		def: def, requiredSlots: required, allowedSlots: allowed,
		outcomes: map[OutcomeKey]struct{}{},
	}
	if def.Resolution != nil {
		if def.Resolution.PendingTTL <= 0 {
			return nil, invalidDefinition("resolution.pending_ttl", "must be positive")
		}
		for _, outcome := range def.Resolution.Outcomes {
			outcome = OutcomeKey(strings.TrimSpace(string(outcome)))
			if !definitionKeyPattern.MatchString(string(outcome)) {
				return nil, invalidDefinition("resolution.outcomes", "contains an invalid outcome")
			}
			if _, exists := action.outcomes[outcome]; exists {
				return nil, invalidDefinition("resolution.outcomes", "contains duplicate %q", outcome)
			}
			action.outcomes[outcome] = struct{}{}
		}
		if len(action.outcomes) == 0 {
			return nil, invalidDefinition("resolution.outcomes", "must not be empty")
		}
		if _, ok := action.outcomes[def.Resolution.DefaultOutcome]; !ok {
			return nil, invalidDefinition("resolution.default_outcome", "must be one of outcomes")
		}
	}
	for index, meterDef := range def.Meters {
		meter, err := compileMeter(meterDef, allowed, action.outcomes, def.Resolution != nil)
		if err != nil {
			return nil, prefixDefinition(err, "meters", index)
		}
		if _, exists := policies[meter.def.ID]; exists {
			return nil, invalidDefinition("meters", "policy %q is reused", meter.def.ID)
		}
		policies[meter.def.ID] = struct{}{}
		action.meters = append(action.meters, meter)
	}
	for _, slot := range required {
		found := false
		for _, meter := range action.meters {
			if meter.def.Slot == slot {
				found = true
				break
			}
		}
		if !found {
			return nil, invalidDefinition("required", "slot %q has no meter", slot)
		}
	}
	if def.Resolution != nil {
		for _, meter := range action.meters {
			if meter.def.Mode == MeterOutcome && def.Resolution.PendingTTL > meter.def.Algorithm.Window {
				return nil, invalidDefinition("resolution.pending_ttl", "must not exceed an outcome meter window")
			}
		}
	}
	if def.Challenge == nil {
		for _, meter := range action.meters {
			if meter.def.ChallengeAt > 0 {
				return nil, invalidDefinition("challenge", "is required when a meter has challenge_at")
			}
		}
	} else {
		def.Challenge.Kind = ChallengeKind(strings.TrimSpace(string(def.Challenge.Kind)))
		if !definitionKeyPattern.MatchString(string(def.Challenge.Kind)) {
			return nil, invalidDefinition("challenge.kind", "must be a stable lowercase key")
		}
		if strings.TrimSpace(def.Challenge.ExpectedAction) == "" {
			return nil, invalidDefinition("challenge.expected_action", "is required")
		}
		if len(def.Challenge.AllowedHosts) == 0 {
			return nil, invalidDefinition("challenge.allowed_hosts", "must not be empty")
		}
		action.def.Challenge = def.Challenge
	}
	return action, nil
}

func compileRequirements(value SignalRequirements) ([]SlotKey, map[SlotKey]Requiredness, error) {
	allowed := map[SlotKey]Requiredness{
		SlotNetwork: value.Network, SlotActor: value.Actor, SlotTarget: value.Target,
	}
	var required []SlotKey
	for slot, requirement := range allowed {
		if requirement == "" {
			requirement = Optional
			allowed[slot] = requirement
		}
		if requirement != Optional && requirement != Required {
			return nil, nil, invalidDefinition("required", "slot %q has unknown requiredness", slot)
		}
		if requirement == Required {
			required = append(required, slot)
		}
	}
	for _, extra := range value.Extra {
		extra.Slot = SlotKey(strings.TrimSpace(string(extra.Slot)))
		if !definitionKeyPattern.MatchString(string(extra.Slot)) {
			return nil, nil, invalidDefinition("required.extra", "contains an invalid slot")
		}
		if _, exists := allowed[extra.Slot]; exists {
			return nil, nil, invalidDefinition("required.extra", "contains duplicate slot %q", extra.Slot)
		}
		requirement := extra.Required
		if requirement == "" {
			requirement = Optional
		}
		if requirement != Optional && requirement != Required {
			return nil, nil, invalidDefinition("required.extra", "slot %q has unknown requiredness", extra.Slot)
		}
		allowed[extra.Slot] = requirement
		if requirement == Required {
			required = append(required, extra.Slot)
		}
	}
	slices.Sort(required)
	return required, allowed, nil
}

func compileMeter(def MeterDefinition, allowed map[SlotKey]Requiredness, outcomes map[OutcomeKey]struct{}, hasResolution bool) (compiledMeter, error) {
	def.ID = PolicyID(strings.TrimSpace(string(def.ID)))
	if !definitionKeyPattern.MatchString(string(def.ID)) {
		return compiledMeter{}, invalidDefinition("id", "must be a stable lowercase key")
	}
	if def.Revision == 0 {
		def.Revision = 1
	}
	if _, ok := allowed[def.Slot]; !ok {
		return compiledMeter{}, invalidDefinition("slot", "%q is not declared by the action", def.Slot)
	}
	if def.Mode == "" {
		def.Mode = MeterAdmission
	}
	if def.Mode != MeterAdmission && def.Mode != MeterOutcome {
		return compiledMeter{}, invalidDefinition("mode", "is unknown")
	}
	if def.Cost <= 0 {
		def.Cost = 1
	}
	if err := validateAlgorithm(def.Algorithm, def.Cost); err != nil {
		return compiledMeter{}, err
	}
	if def.ChallengeAt < 0 || def.ChallengeAt > def.Algorithm.Capacity {
		return compiledMeter{}, invalidDefinition("challenge_at", "must be zero or between 1 and capacity")
	}
	var minimumRetention time.Duration
	switch def.Algorithm.Kind {
	case AlgorithmTokenBucket:
		steps := (def.Algorithm.Capacity + def.Algorithm.RefillAmount - 1) / def.Algorithm.RefillAmount
		if steps > 0 && def.Algorithm.RefillPeriod > 365*24*time.Hour/time.Duration(steps) {
			return compiledMeter{}, invalidDefinition("algorithm", "full refill horizon must not exceed 365 days")
		}
		minimumRetention = time.Duration(steps) * def.Algorithm.RefillPeriod
	default:
		minimumRetention = def.Algorithm.Window
	}
	if def.Retention == 0 {
		switch def.Algorithm.Kind {
		case AlgorithmTokenBucket:
			def.Retention = minimumRetention
		default:
			def.Retention = def.Algorithm.Window
		}
	}
	if def.Retention < minimumRetention {
		return compiledMeter{}, invalidDefinition("retention", "must cover the complete decision horizon")
	}
	meter := compiledMeter{def: def, chargeOn: map[OutcomeKey]bool{}, resetOn: map[OutcomeKey]bool{}}
	if def.Mode == MeterOutcome {
		if !hasResolution {
			return compiledMeter{}, invalidDefinition("mode", "outcome meter requires resolution")
		}
		if def.Algorithm.Kind != AlgorithmSlidingWindow {
			return compiledMeter{}, invalidDefinition("algorithm", "outcome meter requires exact sliding_window")
		}
		if len(def.ChargeOn) == 0 {
			return compiledMeter{}, invalidDefinition("charge_on", "outcome meter requires at least one outcome")
		}
		for _, outcome := range def.ChargeOn {
			if _, ok := outcomes[outcome]; !ok {
				return compiledMeter{}, invalidDefinition("charge_on", "contains unknown outcome %q", outcome)
			}
			meter.chargeOn[outcome] = true
		}
		for _, outcome := range def.ResetOn {
			if _, ok := outcomes[outcome]; !ok {
				return compiledMeter{}, invalidDefinition("reset_on", "contains unknown outcome %q", outcome)
			}
			meter.resetOn[outcome] = true
		}
	} else if len(def.ChargeOn) > 0 || len(def.ResetOn) > 0 {
		return compiledMeter{}, invalidDefinition("mode", "admission meter cannot declare outcome effects")
	}
	return meter, nil
}

func validateAlgorithm(def AlgorithmDefinition, cost int64) error {
	if def.Capacity <= 0 || def.Capacity > 1_000_000 {
		return invalidDefinition("algorithm.capacity", "must be between 1 and 1000000")
	}
	if cost > def.Capacity {
		return invalidDefinition("cost", "must not exceed capacity")
	}
	switch def.Kind {
	case AlgorithmTokenBucket:
		if def.RefillAmount <= 0 || def.RefillAmount > def.Capacity {
			return invalidDefinition("algorithm.refill_amount", "must be between 1 and capacity")
		}
		if def.RefillPeriod <= 0 || def.RefillPeriod > 365*24*time.Hour {
			return invalidDefinition("algorithm.refill_period", "must be positive and at most 365 days")
		}
		if def.Window != 0 {
			return invalidDefinition("algorithm.window", "must be zero for token_bucket")
		}
	case AlgorithmSlidingWindow, AlgorithmFixedWindow:
		if def.Window <= 0 || def.Window > 365*24*time.Hour {
			return invalidDefinition("algorithm.window", "must be positive and at most 365 days")
		}
		if def.RefillAmount != 0 || def.RefillPeriod != 0 {
			return invalidDefinition("algorithm", "refill fields are only valid for token_bucket")
		}
	default:
		return invalidDefinition("algorithm.kind", "is unknown")
	}
	return nil
}

func normalizeLimits(value Limits) (Limits, error) {
	if value.MaxAttemptIDBytes == 0 {
		value.MaxAttemptIDBytes = 200
	}
	if value.MaxSignalBytes == 0 {
		value.MaxSignalBytes = 512
	}
	if value.MaxExtraSignals == 0 {
		value.MaxExtraSignals = 8
	}
	if value.MaxProofBytes == 0 {
		value.MaxProofBytes = 2048
	}
	if value.ReceiptRetention == 0 {
		value.ReceiptRetention = 7 * 24 * time.Hour
	}
	if value.MaxAttemptIDBytes < 16 || value.MaxAttemptIDBytes > 1024 {
		return Limits{}, invalidDefinition("limits.max_attempt_id_bytes", "must be between 16 and 1024")
	}
	if value.MaxSignalBytes < 1 || value.MaxSignalBytes > 4096 {
		return Limits{}, invalidDefinition("limits.max_signal_bytes", "must be between 1 and 4096")
	}
	if value.MaxExtraSignals < 0 || value.MaxExtraSignals > 32 {
		return Limits{}, invalidDefinition("limits.max_extra_signals", "must be between 0 and 32")
	}
	if value.MaxProofBytes < 1 || value.MaxProofBytes > 8192 {
		return Limits{}, invalidDefinition("limits.max_proof_bytes", "must be between 1 and 8192")
	}
	if value.ReceiptRetention <= 0 {
		return Limits{}, invalidDefinition("limits.receipt_retention", "must be positive")
	}
	return value, nil
}

func prefixDefinition(err error, field string, index int) error {
	typed, ok := err.(*Error)
	if !ok {
		return err
	}
	copy := *typed
	if copy.Field == "" {
		copy.Field = field
	} else {
		copy.Field = field + "[" + jsonNumber(index) + "]." + copy.Field
	}
	return &copy
}

func jsonNumber(value int) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func (catalog *Catalog) Version() uint64  { return catalog.version }
func (catalog *Catalog) Consumer() string { return catalog.consumer }
func (catalog *Catalog) Digest() string   { return catalog.digest }
func (catalog *Catalog) Limits() Limits   { return catalog.limits }
