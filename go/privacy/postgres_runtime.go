package privacy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lib/pq"
)

type PostgresOptions struct {
	DB          *sql.DB
	InstanceKey string
	Clock       func() time.Time
}

type PostgresRuntime struct {
	db          *sql.DB
	tx          *sql.Tx
	instanceKey string
	catalog     *Catalog
	clock       func() time.Time
}

var _ Runtime = (*PostgresRuntime)(nil)
var _ EvidenceLedger = (*PostgresRuntime)(nil)
var _ RetentionLedger = (*PostgresRuntime)(nil)

func NewPostgresRuntime(ctx context.Context, catalog *Catalog, options PostgresOptions) (*PostgresRuntime, error) {
	if catalog == nil {
		return nil, invalid("catalog", "is required")
	}
	if options.DB == nil {
		return nil, invalid("db", "is required")
	}
	instanceKey := strings.TrimSpace(options.InstanceKey)
	if instanceKey == "" || len(instanceKey) > 200 || strings.ContainsRune(instanceKey, '\x00') {
		return nil, invalid("instance_key", "is invalid")
	}
	if err := options.DB.PingContext(ctx); err != nil {
		return nil, postgresError("ping database", err)
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	runtime := &PostgresRuntime{db: options.DB, instanceKey: instanceKey, catalog: catalog, clock: clock}
	if err := runtime.bootstrap(ctx); err != nil {
		return nil, err
	}
	return runtime, nil
}

// Bind returns a transaction-bound view. It never commits or rolls back tx.
func (runtime *PostgresRuntime) Bind(tx *sql.Tx) *PostgresRuntime {
	if runtime == nil || tx == nil {
		return nil
	}
	copy := *runtime
	copy.tx = tx
	return &copy
}

func (runtime *PostgresRuntime) Evidence() EvidenceLedger   { return runtime }
func (runtime *PostgresRuntime) Retention() RetentionLedger { return runtime }

func (runtime *PostgresRuntime) Purpose(key PurposeKey) (Processing, error) {
	ref, exists := runtime.catalog.active[key]
	if !exists {
		return nil, &Error{Kind: ErrorNotFound, Field: "purpose", Message: fmt.Sprintf("%q is not active", key)}
	}
	return &postgresProcessing{runtime: runtime, ref: ref}, nil
}

type postgresProcessing struct {
	runtime *PostgresRuntime
	ref     PurposeRef
}

func (processing *postgresProcessing) Ref() PurposeRef { return processing.ref }
func (processing *postgresProcessing) Decide(ctx context.Context, input DecisionInput) (ProcessingDecision, error) {
	return processing.runtime.decidePostgres(ctx, processing.ref, input)
}

type privacyQueryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (runtime *PostgresRuntime) queryer() privacyQueryer {
	if runtime.tx != nil {
		return runtime.tx
	}
	return runtime.db
}

func (runtime *PostgresRuntime) bootstrap(ctx context.Context) error {
	tx, err := runtime.db.BeginTx(ctx, nil)
	if err != nil {
		return postgresError("begin bootstrap", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := runtime.clock().UTC()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO privacy_instances(instance_key, schema_version, catalog_version, catalog_digest, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$5)
ON CONFLICT (instance_key) DO NOTHING`,
		runtime.instanceKey, CurrentSchemaVersion, runtime.catalog.version, runtime.catalog.digest, now,
	); err != nil {
		return postgresError("bootstrap instance", err)
	}
	var schemaVersion int64
	if err := tx.QueryRowContext(ctx, `
SELECT schema_version FROM privacy_instances WHERE instance_key=$1 FOR UPDATE`,
		runtime.instanceKey,
	).Scan(&schemaVersion); err != nil {
		return postgresError("load instance", err)
	}
	if schemaVersion != int64(CurrentSchemaVersion) {
		return &Error{Kind: ErrorStoreUnavailable, Field: "schema_version", Message: fmt.Sprintf("database has %d, module requires %d", schemaVersion, CurrentSchemaVersion)}
	}
	if err := runtime.reconcileDefinitions(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE privacy_instances SET catalog_version=$2, catalog_digest=$3, updated_at=$4 WHERE instance_key=$1`,
		runtime.instanceKey, runtime.catalog.version, runtime.catalog.digest, now,
	); err != nil {
		return postgresError("update instance catalog", err)
	}
	if err := tx.Commit(); err != nil {
		return postgresError("commit bootstrap", err)
	}
	return nil
}

type definitionRevisionRecord struct {
	kind     string
	key      string
	revision Revision
	value    any
}

func (runtime *PostgresRuntime) reconcileDefinitions(ctx context.Context, tx *sql.Tx) error {
	var records []definitionRevisionRecord
	for ref, value := range runtime.catalog.notices {
		records = append(records, definitionRevisionRecord{"notice", string(ref.Key), ref.Revision, value})
	}
	for ref, value := range runtime.catalog.purposes {
		records = append(records, definitionRevisionRecord{"purpose", string(ref.Key), ref.Revision, value})
	}
	for ref, value := range runtime.catalog.retention {
		records = append(records, definitionRevisionRecord{"retention", string(ref.Key), ref.Revision, value})
	}
	if runtime.catalog.owner != nil {
		records = append(records, definitionRevisionRecord{"owner", string(runtime.catalog.owner.Ref.Key), runtime.catalog.owner.Ref.Revision, *runtime.catalog.owner})
	}
	for _, value := range runtime.catalog.owners {
		records = append(records, definitionRevisionRecord{"owner", string(value.Ref.Key), value.Ref.Revision, value})
	}
	for _, record := range records {
		encoded, _ := json.Marshal(record.value)
		sum := sha256.Sum256(encoded)
		digest := hex.EncodeToString(sum[:])
		var stored string
		err := tx.QueryRowContext(ctx, `
SELECT digest FROM privacy_definition_revisions
WHERE instance_key=$1 AND definition_kind=$2 AND definition_key=$3 AND revision=$4`,
			runtime.instanceKey, record.kind, record.key, record.revision,
		).Scan(&stored)
		switch {
		case err == nil && stored != digest:
			return &Error{Kind: ErrorDefinitionDrift, Field: record.kind, Message: fmt.Sprintf("%s revision %d changed digest", record.key, record.revision)}
		case err == nil:
			continue
		case !errors.Is(err, sql.ErrNoRows):
			return postgresError("load definition revision", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO privacy_definition_revisions(
    instance_key, definition_kind, definition_key, revision, digest, definition
) VALUES ($1,$2,$3,$4,$5,$6)`,
			runtime.instanceKey, record.kind, record.key, record.revision, digest, encoded,
		); err != nil {
			return postgresError("insert definition revision", err)
		}
	}
	return nil
}

func (runtime *PostgresRuntime) Consent(ctx context.Context, command ConsentCommand) (ConsentReceipt, error) {
	now := runtime.clock().UTC()
	validator := &Memory{catalog: runtime.catalog, clock: runtime.clock}
	prepared, err := validator.prepareConsent(command, now)
	if err != nil {
		return ConsentReceipt{}, err
	}
	fingerprintValue := fingerprint(prepared)
	var result ConsentReceipt
	err = runtime.mutate(ctx, func(queryer privacyQueryer) error {
		existing, found, err := loadPostgresCommand[ConsentReceipt](ctx, queryer, runtime.instanceKey, prepared.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if existing.kind != "consent" || existing.fingerprint != fingerprintValue {
				return conflict("idempotency_key", "is reused with a different command")
			}
			result = existing.receipt
			result.Replay = true
			return nil
		}
		result = ConsentReceipt{
			ID: receiptID("consent", fingerprintValue), Subject: prepared.Subject, Notice: prepared.Notice,
			Purposes: append([]PurposeRef(nil), prepared.Purposes...), OccurredAt: prepared.OccurredAt,
			RecordedAt: now, Channel: prepared.Channel, EvidenceDigest: prepared.EvidenceDigest,
			Fingerprint: fingerprintValue,
		}
		return insertEvidence(ctx, queryer, runtime.instanceKey, prepared.IdempotencyKey, "consent",
			fingerprintValue, result.ID, prepared.Subject, prepared.Purposes, "", prepared.OccurredAt, nil, result)
	})
	return result, err
}

func (runtime *PostgresRuntime) Withdraw(ctx context.Context, command WithdrawalCommand) (WithdrawalReceipt, error) {
	now := runtime.clock().UTC()
	validator := &Memory{catalog: runtime.catalog, clock: runtime.clock}
	prepared, err := validator.prepareWithdrawal(command, now)
	if err != nil {
		return WithdrawalReceipt{}, err
	}
	fingerprintValue := fingerprint(prepared)
	var result WithdrawalReceipt
	err = runtime.mutate(ctx, func(queryer privacyQueryer) error {
		existing, found, err := loadPostgresCommand[WithdrawalReceipt](ctx, queryer, runtime.instanceKey, prepared.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if existing.kind != "withdrawal" || existing.fingerprint != fingerprintValue {
				return conflict("idempotency_key", "is reused with a different command")
			}
			result = existing.receipt
			result.Replay = true
			return nil
		}
		supersedes, err := findSupersededConsents(ctx, queryer, runtime.instanceKey, prepared)
		if err != nil {
			return err
		}
		result = WithdrawalReceipt{
			ID: receiptID("withdrawal", fingerprintValue), Subject: prepared.Subject,
			Purposes: append([]PurposeRef(nil), prepared.Purposes...), Supersedes: supersedes,
			OccurredAt: prepared.OccurredAt, RecordedAt: now, Channel: prepared.Channel,
			Reason: prepared.Reason, Fingerprint: fingerprintValue,
		}
		return insertEvidence(ctx, queryer, runtime.instanceKey, prepared.IdempotencyKey, "withdrawal",
			fingerprintValue, result.ID, prepared.Subject, prepared.Purposes, "", prepared.OccurredAt, nil, result)
	})
	return result, err
}

func (runtime *PostgresRuntime) ObserveSignal(ctx context.Context, command SignalCommand) (SignalReceipt, error) {
	now := runtime.clock().UTC()
	validator := &Memory{catalog: runtime.catalog, clock: runtime.clock}
	prepared, err := validator.prepareSignal(command, now)
	if err != nil {
		return SignalReceipt{}, err
	}
	fingerprintValue := fingerprint(prepared)
	var result SignalReceipt
	err = runtime.mutate(ctx, func(queryer privacyQueryer) error {
		existing, found, err := loadPostgresCommand[SignalReceipt](ctx, queryer, runtime.instanceKey, prepared.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if existing.kind != "signal" || existing.fingerprint != fingerprintValue {
				return conflict("idempotency_key", "is reused with a different command")
			}
			result = existing.receipt
			result.Replay = true
			return nil
		}
		result = SignalReceipt{
			ID: receiptID("signal", fingerprintValue), Subject: prepared.Subject, Signal: prepared.Signal,
			AssertedAt: prepared.AssertedAt, ExpiresAt: prepared.ExpiresAt, RecordedAt: now,
			Channel: prepared.Channel, Fingerprint: fingerprintValue,
		}
		return insertEvidence(ctx, queryer, runtime.instanceKey, prepared.IdempotencyKey, "signal",
			fingerprintValue, result.ID, prepared.Subject, nil, prepared.Signal, prepared.AssertedAt, prepared.ExpiresAt, result)
	})
	return result, err
}

type postgresCommand[T any] struct {
	kind        string
	fingerprint string
	receipt     T
}

func loadPostgresCommand[T any](
	ctx context.Context, queryer privacyQueryer, instanceKey string, key IdempotencyKey,
) (postgresCommand[T], bool, error) {
	var result postgresCommand[T]
	var encoded []byte
	err := queryer.QueryRowContext(ctx, `
SELECT command_kind, fingerprint, receipt
FROM privacy_command_receipts WHERE instance_key=$1 AND idempotency_key=$2`,
		instanceKey, key,
	).Scan(&result.kind, &result.fingerprint, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return result, false, nil
	}
	if err != nil {
		return result, false, postgresError("load command receipt", err)
	}
	if err := json.Unmarshal(encoded, &result.receipt); err != nil {
		return result, false, postgresError("decode command receipt", err)
	}
	return result, true, nil
}

func insertEvidence(
	ctx context.Context, queryer privacyQueryer, instanceKey string, key IdempotencyKey,
	kind, fingerprintValue string, receiptID ReceiptID, subject SubjectRef, purposes []PurposeRef,
	signal SignalKey, occurredAt time.Time, expiresAt *time.Time, receipt any,
) error {
	encoded, _ := json.Marshal(receipt)
	purposeJSON, _ := json.Marshal(purposes)
	if _, err := queryer.ExecContext(ctx, `
INSERT INTO privacy_command_receipts(
    instance_key, idempotency_key, command_kind, fingerprint, receipt, recorded_at
) VALUES ($1,$2,$3,$4,$5,$6)`,
		instanceKey, key, kind, fingerprintValue, encoded, occurredAt,
	); err != nil {
		return postgresError("insert command receipt", err)
	}
	if _, err := queryer.ExecContext(ctx, `
INSERT INTO privacy_evidence_events(
    instance_key, receipt_id, event_kind, subject_owner, subject_kind, subject_value,
    purposes, signal_key, occurred_at, expires_at, receipt
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		instanceKey, receiptID, kind, subject.Owner, subject.Kind, subject.Value,
		purposeJSON, signal, occurredAt, expiresAt, encoded,
	); err != nil {
		return postgresError("insert evidence event", err)
	}
	return nil
}

func findSupersededConsents(
	ctx context.Context, queryer privacyQueryer, instanceKey string, command WithdrawalCommand,
) ([]ReceiptID, error) {
	rows, err := queryer.QueryContext(ctx, `
SELECT receipt_id, purposes
FROM privacy_evidence_events
WHERE instance_key=$1 AND event_kind='consent'
  AND subject_owner=$2 AND subject_kind=$3 AND subject_value=$4 AND occurred_at <= $5`,
		instanceKey, command.Subject.Owner, command.Subject.Kind, command.Subject.Value, command.OccurredAt,
	)
	if err != nil {
		return nil, postgresError("query superseded consents", err)
	}
	defer rows.Close()
	var result []ReceiptID
	for rows.Next() {
		var id ReceiptID
		var encoded []byte
		if err := rows.Scan(&id, &encoded); err != nil {
			return nil, postgresError("scan superseded consent", err)
		}
		var purposes []PurposeRef
		if err := json.Unmarshal(encoded, &purposes); err != nil {
			return nil, postgresError("decode superseded consent", err)
		}
		for _, purpose := range command.Purposes {
			if slices.Contains(purposes, purpose) {
				result = append(result, id)
				break
			}
		}
	}
	slices.Sort(result)
	return result, rows.Err()
}

func (runtime *PostgresRuntime) decidePostgres(ctx context.Context, ref PurposeRef, input DecisionInput) (ProcessingDecision, error) {
	purpose, exists := runtime.catalog.purposes[ref]
	if !exists {
		return ProcessingDecision{}, &Error{Kind: ErrorNotFound, Field: "purpose", Message: "is not defined"}
	}
	at := input.At
	if at.IsZero() {
		at = runtime.clock()
	}
	at = at.UTC()
	decision := ProcessingDecision{
		Kind: DecisionDeny, Purpose: ref, Basis: purpose.Basis, Reasons: []ReasonCode{"purpose_unavailable"},
		CatalogDigest: runtime.catalog.digest, DecidedAt: at,
	}
	if (!purpose.EffectiveAt.IsZero() && at.Before(purpose.EffectiveAt)) ||
		(purpose.RetiredAt != nil && !at.Before(*purpose.RetiredAt)) {
		return decision, nil
	}
	subjects := normalizedSubjects(input.Subject)
	signals := append([]ObservedSignal(nil), input.Signals...)
	for _, subject := range subjects {
		rows, err := runtime.queryer().QueryContext(ctx, `
SELECT signal_key, occurred_at
FROM privacy_evidence_events
WHERE instance_key=$1 AND event_kind='signal'
  AND subject_owner=$2 AND subject_kind=$3 AND subject_value=$4
  AND occurred_at <= $5 AND (expires_at IS NULL OR expires_at > $5)`,
			runtime.instanceKey, subject.Owner, subject.Kind, subject.Value, at,
		)
		if err != nil {
			return ProcessingDecision{}, postgresError("query privacy signals", err)
		}
		for rows.Next() {
			var value ObservedSignal
			if err := rows.Scan(&value.Signal, &value.AssertedAt); err != nil {
				_ = rows.Close()
				return ProcessingDecision{}, postgresError("scan privacy signal", err)
			}
			signals = append(signals, value)
		}
		if err := rows.Close(); err != nil {
			return ProcessingDecision{}, postgresError("close privacy signals", err)
		}
	}
	for _, rule := range purpose.SignalRules {
		if observesSignal(signals, rule.Signal, at) {
			if rule.Effect == SignalDeny {
				decision.Reasons = []ReasonCode{"privacy_signal"}
				return decision, nil
			}
			decision.Kind, decision.Reasons = DecisionRestrict, []ReasonCode{"privacy_signal"}
			decision.Restrictions = append([]RestrictionKey(nil), rule.Restrictions...)
			return decision, nil
		}
	}
	if purpose.Basis != BasisConsent {
		decision.Kind, decision.Reasons = DecisionAllow, []ReasonCode{"declared_non_consent_basis"}
		return decision, nil
	}
	var latest memoryEvidence
	latestKind := ""
	for _, subject := range subjects {
		rows, err := runtime.queryer().QueryContext(ctx, `
SELECT event_kind, receipt_id, purposes, occurred_at, receipt
FROM privacy_evidence_events
WHERE instance_key=$1 AND event_kind IN ('consent','withdrawal')
  AND subject_owner=$2 AND subject_kind=$3 AND subject_value=$4 AND occurred_at <= $5
ORDER BY occurred_at DESC, receipt_id DESC`,
			runtime.instanceKey, subject.Owner, subject.Kind, subject.Value, at,
		)
		if err != nil {
			return ProcessingDecision{}, postgresError("query consent evidence", err)
		}
		for rows.Next() {
			var event memoryEvidence
			var encodedPurposes, encodedReceipt []byte
			if err := rows.Scan(&event.kind, &event.id, &encodedPurposes, &event.occurredAt, &encodedReceipt); err != nil {
				_ = rows.Close()
				return ProcessingDecision{}, postgresError("scan consent evidence", err)
			}
			if err := json.Unmarshal(encodedPurposes, &event.purposes); err != nil {
				_ = rows.Close()
				return ProcessingDecision{}, postgresError("decode consent purposes", err)
			}
			if !slices.Contains(event.purposes, ref) {
				continue
			}
			if latestKind == "" || event.occurredAt.After(latest.occurredAt) ||
				(event.occurredAt.Equal(latest.occurredAt) && string(event.id) > string(latest.id)) {
				latestKind, latest = event.kind, event
			}
		}
		if err := rows.Close(); err != nil {
			return ProcessingDecision{}, postgresError("close consent evidence", err)
		}
	}
	switch latestKind {
	case "consent":
		decision.Kind, decision.Reasons, decision.Evidence = DecisionAllow, []ReasonCode{"affirmative_consent"}, []ReceiptID{latest.id}
	case "withdrawal":
		decision.Reasons = []ReasonCode{"consent_withdrawn"}
	default:
		decision.Reasons = []ReasonCode{"consent_missing"}
	}
	return decision, nil
}

func normalizedSubjects(context SubjectContext) []SubjectRef {
	var result []SubjectRef
	if context.Current != nil {
		result = append(result, *context.Current)
	}
	result = append(result, context.Aliases...)
	slices.SortFunc(result, func(a, b SubjectRef) int {
		return strings.Compare(string(a.Owner)+"\x00"+string(a.Kind)+"\x00"+a.Value, string(b.Owner)+"\x00"+string(b.Kind)+"\x00"+b.Value)
	})
	return slices.Compact(result)
}

func (runtime *PostgresRuntime) mutate(ctx context.Context, operation func(privacyQueryer) error) error {
	if runtime.tx != nil {
		return operation(runtime.tx)
	}
	tx, err := runtime.db.BeginTx(ctx, nil)
	if err != nil {
		return postgresError("begin transaction", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := operation(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return postgresError("commit transaction", err)
	}
	return nil
}

func postgresError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var pqError *pq.Error
	retryable := false
	if errors.As(err, &pqError) {
		switch string(pqError.Code) {
		case "55P03", "40P01", "40001":
			retryable = true
		}
	}
	return &Error{Kind: ErrorStoreUnavailable, Field: "postgres", Message: operation + " failed", Retryable: retryable, Cause: err}
}
