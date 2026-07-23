package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
)

type Attempt[E any] struct {
	ID          EventID
	Actor       Actor
	Target      Target
	Outcome     Outcome
	Correlation Correlation
	OccurredAt  time.Time
	Evidence    E
}

type Encoder[E any] func(E) []EvidenceField

type Contract[E any] struct {
	catalog *Catalog
	action  Action
	encode  Encoder[E]
}

type preparedEvent struct {
	ID               EventID
	Action           Action
	Actor            Actor
	Target           Target
	Outcome          Outcome
	Correlation      Correlation
	OccurredAt       time.Time
	Evidence         []EvidenceField
	RetentionClass   RetentionClass
	DefinitionDigest Digest
	Commit           CommitRequirement
	Fingerprint      Digest
}

type Command struct {
	value preparedEvent
}

type PreparedView struct {
	ID               EventID
	Action           Action
	Actor            Actor
	Target           Target
	Outcome          Outcome
	Correlation      Correlation
	OccurredAt       time.Time
	Evidence         []EvidenceField
	RetentionClass   RetentionClass
	DefinitionDigest Digest
	Commit           CommitRequirement
	Fingerprint      Digest
}

func BindAction[E any](catalog *Catalog, action Action, encode Encoder[E]) (Contract[E], error) {
	if catalog == nil {
		return Contract[E]{}, invalidDefinition("catalog", "is required")
	}
	if encode == nil {
		return Contract[E]{}, invalidDefinition("encoder", "is required")
	}
	if _, exists := catalog.actions[action]; !exists {
		return Contract[E]{}, &Error{Kind: ErrorUnknownAction, Field: "action", Message: "is not declared"}
	}
	return Contract[E]{catalog: catalog, action: action, encode: encode}, nil
}

func MustBindAction[E any](catalog *Catalog, action Action, encode Encoder[E]) Contract[E] {
	contract, err := BindAction(catalog, action, encode)
	if err != nil {
		panic(err)
	}
	return contract
}

func Prepare[E any](contract Contract[E], attempt Attempt[E]) (Command, error) {
	if contract.catalog == nil || contract.encode == nil {
		return Command{}, invalidAttempt("contract", "is not initialized")
	}
	fields := contract.encode(attempt.Evidence)
	return contract.catalog.prepare(contract.action, attempt.ID, attempt.Actor, attempt.Target, attempt.Outcome, attempt.Correlation, attempt.OccurredAt, fields)
}

func Record[E any](
	ctx Context,
	appender Appender,
	contract Contract[E],
	attempt Attempt[E],
) (Event, error) {
	command, err := Prepare(contract, attempt)
	if err != nil {
		return Event{}, err
	}
	return appender.Append(ctx, command)
}

func DeriveEventID(commandID string, action Action, ordinal uint32) EventID {
	raw := fmt.Sprintf("%s\x00%s\x00%d\x00%d", strings.TrimSpace(commandID), action.Name, action.Version, ordinal)
	sum := sha256.Sum256([]byte(raw))
	return EventID(hex.EncodeToString(sum[:]))
}

func (command Command) View() PreparedView {
	value := command.value
	return PreparedView{
		ID: value.ID, Action: value.Action, Actor: value.Actor, Target: value.Target,
		Outcome: value.Outcome, Correlation: value.Correlation, OccurredAt: value.OccurredAt,
		Evidence: cloneEvidence(value.Evidence), RetentionClass: value.RetentionClass,
		DefinitionDigest: value.DefinitionDigest, Commit: value.Commit, Fingerprint: value.Fingerprint,
	}
}

func (catalog *Catalog) prepare(
	action Action,
	id EventID,
	actor Actor,
	target Target,
	outcome Outcome,
	correlation Correlation,
	occurredAt time.Time,
	evidence []EvidenceField,
) (Command, error) {
	policy, exists := catalog.actions[action]
	if !exists {
		return Command{}, &Error{Kind: ErrorUnknownAction, Field: "action", Message: "is not declared"}
	}
	if strings.TrimSpace(string(id)) == "" || len(id) > 128 || !codePattern.MatchString(string(id)) {
		return Command{}, invalidAttempt("id", "must be a stable code-like value up to 128 characters")
	}
	if !validActor(actor) {
		return Command{}, invalidAttempt("actor", "is invalid")
	}
	if !slices.Contains(policy.definition.TargetTypes, target.Type) || strings.TrimSpace(target.ID) == "" || len(target.ID) > 256 {
		return Command{}, invalidAttempt("target", "does not match the Action contract")
	}
	if outcome.Kind != OutcomeSucceeded && outcome.Kind != OutcomeDenied && outcome.Kind != OutcomeFailed {
		return Command{}, invalidAttempt("outcome.kind", "is invalid")
	}
	if outcome.Kind != OutcomeSucceeded && (outcome.Reason == "" || !codePattern.MatchString(string(outcome.Reason))) {
		return Command{}, invalidAttempt("outcome.reason", "is required for denied and failed outcomes")
	}
	if outcome.Kind == OutcomeSucceeded && outcome.Reason != "" && !codePattern.MatchString(string(outcome.Reason)) {
		return Command{}, invalidAttempt("outcome.reason", "is invalid")
	}
	if err := validateCorrelation(correlation); err != nil {
		return Command{}, err
	}
	normalized, err := catalog.normalizeEvidence(policy, evidence)
	if err != nil {
		return Command{}, err
	}
	value := preparedEvent{
		ID: id, Action: action, Actor: actor, Target: target, Outcome: outcome,
		Correlation: correlation, OccurredAt: occurredAt.UTC(), Evidence: normalized,
		RetentionClass: policy.definition.Retention, DefinitionDigest: catalog.digest,
		Commit: policy.definition.Commit,
	}
	fingerprint, err := preparedFingerprint(value)
	if err != nil {
		return Command{}, invalidAttempt("attempt", "cannot be encoded")
	}
	value.Fingerprint = fingerprint
	return Command{value: value}, nil
}

func (catalog *Catalog) normalizeEvidence(policy compiledAction, in []EvidenceField) ([]EvidenceField, error) {
	if len(in) > catalog.definition.MaxEvidence {
		return nil, rejectedEvidence("evidence", "contains too many fields")
	}
	out := make([]EvidenceField, 0, len(in))
	seen := make(map[EvidenceKey]struct{}, len(in))
	for _, field := range in {
		definition, exists := policy.fields[field.Key]
		if !exists {
			return nil, rejectedEvidence(string(field.Key), "is not allowed by the Action contract")
		}
		if _, exists := seen[field.Key]; exists {
			return nil, rejectedEvidence(string(field.Key), "is duplicated")
		}
		if field.Kind != definition.Kind {
			return nil, rejectedEvidence(string(field.Key), "has the wrong type")
		}
		if err := validateEvidenceShape(field); err != nil {
			return nil, rejectedEvidence(string(field.Key), err.Error())
		}
		if err := validateEvidenceValue(field, definition); err != nil {
			return nil, rejectedEvidence(string(field.Key), err.Error())
		}
		seen[field.Key] = struct{}{}
		out = append(out, field.clone())
	}
	for key, definition := range policy.fields {
		if definition.Required {
			if _, exists := seen[key]; !exists {
				return nil, rejectedEvidence(string(key), "is required")
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func validateEvidenceValue(field EvidenceField, definition FieldDefinition) error {
	switch field.Kind {
	case EvidenceCode:
		if len(field.Text) > definition.MaxLength || !codePattern.MatchString(field.Text) {
			return fmt.Errorf("must be a bounded code-like value")
		}
	case EvidenceReference:
		if !validReference(field.Text, definition.MaxLength) {
			return fmt.Errorf("must be a bounded stable reference")
		}
	case EvidenceDigest:
		if !hexDigestPattern.MatchString(field.Text) {
			return fmt.Errorf("must be a lowercase SHA-256 digest")
		}
	case EvidenceTime:
		if field.Time.IsZero() {
			return fmt.Errorf("must not be zero")
		}
	case EvidenceCodeList:
		if len(field.List) > definition.MaxItems {
			return fmt.Errorf("contains too many items")
		}
		for _, value := range field.List {
			if len(value) > definition.MaxLength || !codePattern.MatchString(value) {
				return fmt.Errorf("contains an invalid code")
			}
		}
	case EvidenceReferenceList:
		if len(field.List) > definition.MaxItems {
			return fmt.Errorf("contains too many items")
		}
		for _, value := range field.List {
			if !validReference(value, definition.MaxLength) {
				return fmt.Errorf("contains an invalid stable reference")
			}
		}
	}
	return nil
}

func validReference(value string, maxLength int) bool {
	return value != "" && len(value) <= maxLength && strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r)
	}) == -1
}

func validActor(actor Actor) bool {
	switch actor.Kind {
	case ActorUser, ActorGuest, ActorService:
		return actor.ID != "" && len(actor.ID) <= 256
	case ActorSystem:
		return actor.ID != "" && len(actor.ID) <= 256
	case ActorAnonymous:
		return actor.ID == ""
	default:
		return false
	}
}

func validateCorrelation(value Correlation) error {
	fields := []struct {
		name  string
		value string
		max   int
	}{
		{"correlation.request_id", value.RequestID, 128},
		{"correlation.causation_id", value.CausationID, 128},
		{"correlation.command_id", value.CommandID, 128},
		{"correlation.batch_id", value.BatchID, 128},
	}
	for _, field := range fields {
		if field.value != "" && (len(field.value) > field.max || !codePattern.MatchString(field.value)) {
			return invalidAttempt(field.name, "must be a bounded code-like value")
		}
	}
	if value.TraceID != "" && (!traceIDPattern.MatchString(value.TraceID) || value.TraceID == strings.Repeat("0", 32)) {
		return invalidAttempt("correlation.trace_id", "must be a non-zero W3C trace ID")
	}
	if value.SpanID != "" {
		if value.TraceID == "" {
			return invalidAttempt("correlation.span_id", "requires trace_id")
		}
		if !spanIDPattern.MatchString(value.SpanID) || value.SpanID == strings.Repeat("0", 16) {
			return invalidAttempt("correlation.span_id", "must be a non-zero W3C span ID")
		}
	}
	return nil
}

func preparedFingerprint(value preparedEvent) (Digest, error) {
	raw, err := json.Marshal(struct {
		ID               EventID
		Action           Action
		Actor            Actor
		Target           Target
		Outcome          Outcome
		Correlation      Correlation
		OccurredAt       time.Time
		Evidence         []EvidenceField
		RetentionClass   RetentionClass
		DefinitionDigest Digest
	}{
		value.ID, value.Action, value.Actor, value.Target, value.Outcome, value.Correlation,
		value.OccurredAt, value.Evidence, value.RetentionClass, value.DefinitionDigest,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return Digest(hex.EncodeToString(sum[:])), nil
}

func cloneEvidence(in []EvidenceField) []EvidenceField {
	out := make([]EvidenceField, len(in))
	for i := range in {
		out[i] = in[i].clone()
	}
	return out
}
