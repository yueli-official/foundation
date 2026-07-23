package abuse

import (
	"bytes"
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
	Secret      []byte
	KeyVersion  uint32
	LockTimeout time.Duration
	Verifiers   map[ChallengeKind]ChallengeVerifier
}

type postgresStore struct {
	db          *sql.DB
	instanceKey string
	catalog     *Catalog
	clock       func() time.Time
	lockTimeout time.Duration
	policies    map[PolicyID]compiledMeter
}

func NewPostgres(ctx context.Context, catalog *Catalog, options PostgresOptions) (Module, error) {
	if catalog == nil {
		return nil, invalidInput("catalog", "is required")
	}
	if options.DB == nil {
		return nil, invalidInput("db", "is required")
	}
	instanceKey := strings.TrimSpace(options.InstanceKey)
	if instanceKey == "" || len(instanceKey) > 200 || strings.ContainsRune(instanceKey, '\x00') {
		return nil, invalidInput("instance_key", "is invalid")
	}
	if err := options.DB.PingContext(ctx); err != nil {
		return nil, unavailable("postgres", "database ping failed", err)
	}
	candidateSecret, err := normalizeSecret(options.Secret)
	if err != nil {
		return nil, err
	}
	keyVersion := options.KeyVersion
	if keyVersion == 0 {
		keyVersion = 1
	}
	lockTimeout := options.LockTimeout
	if lockTimeout == 0 {
		lockTimeout = 2 * time.Second
	}
	if lockTimeout < time.Millisecond || lockTimeout > 30*time.Second {
		return nil, invalidInput("lock_timeout", "must be between 1ms and 30s")
	}
	store := &postgresStore{
		db: options.DB, instanceKey: instanceKey, catalog: catalog,
		clock: options.Clock, lockTimeout: lockTimeout, policies: make(map[PolicyID]compiledMeter),
	}
	for _, action := range catalog.actions {
		for _, meter := range action.meters {
			store.policies[meter.def.ID] = meter
		}
	}
	storedSecret, storedKeyVersion, err := store.bootstrap(ctx, candidateSecret, keyVersion)
	if err != nil {
		return nil, err
	}
	return &runtime{
		catalog: catalog, store: store, secret: storedSecret,
		keyVersion: storedKeyVersion, verifiers: cloneVerifiers(options.Verifiers),
	}, nil
}

func (store *postgresStore) bootstrap(ctx context.Context, candidateSecret [32]byte, candidateKeyVersion uint32) ([32]byte, uint32, error) {
	var zero [32]byte
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return zero, 0, storeError("begin bootstrap", err)
	}
	defer func() { _ = tx.Rollback() }()
	now, err := store.nowTx(ctx, tx)
	if err != nil {
		return zero, 0, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO abuse_instances (
    instance_key, schema_version, consumer, definition_version, definition_digest,
    active_key_version, subject_secret, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
ON CONFLICT (instance_key) DO NOTHING`,
		store.instanceKey, PostgresSchemaVersion, store.catalog.consumer,
		store.catalog.version, store.catalog.digest, candidateKeyVersion,
		candidateSecret[:], now,
	); err != nil {
		return zero, 0, storeError("insert instance", err)
	}
	var (
		schemaVersion int
		consumer      string
		oldVersion    uint64
		oldDigest     string
		keyVersion    uint32
		secret        []byte
	)
	if err := tx.QueryRowContext(ctx, `
SELECT schema_version, consumer, definition_version, definition_digest,
       active_key_version, subject_secret
FROM abuse_instances
WHERE instance_key = $1
FOR UPDATE`, store.instanceKey).Scan(
		&schemaVersion, &consumer, &oldVersion, &oldDigest, &keyVersion, &secret,
	); err != nil {
		return zero, 0, storeError("load instance", err)
	}
	if schemaVersion != PostgresSchemaVersion {
		return zero, 0, &Error{
			Kind: ErrorDefinitionDrift, Field: "schema_version",
			Message: fmt.Sprintf("database has %d, module requires %d", schemaVersion, PostgresSchemaVersion),
		}
	}
	if consumer != store.catalog.consumer {
		return zero, 0, &Error{Kind: ErrorDefinitionDrift, Field: "consumer", Message: "does not match database"}
	}
	if len(secret) != 32 {
		return zero, 0, &Error{Kind: ErrorStoreUnavailable, Field: "subject_secret", Message: "database value is invalid"}
	}
	if oldVersion > store.catalog.version {
		return zero, 0, &Error{Kind: ErrorDefinitionDrift, Field: "version", Message: "definition rollback is not allowed"}
	}
	if oldVersion == store.catalog.version && oldDigest != store.catalog.digest {
		return zero, 0, &Error{Kind: ErrorDefinitionDrift, Field: "definition", Message: "same version has a different digest"}
	}
	if err := store.reconcilePoliciesTx(ctx, tx, oldVersion, oldDigest, now); err != nil {
		return zero, 0, err
	}
	if oldVersion < store.catalog.version {
		if _, err := tx.ExecContext(ctx, `
UPDATE abuse_instances
SET definition_version = $2, definition_digest = $3, updated_at = $4
WHERE instance_key = $1`,
			store.instanceKey, store.catalog.version, store.catalog.digest, now,
		); err != nil {
			return zero, 0, storeError("update instance definition", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return zero, 0, storeError("commit bootstrap", err)
	}
	copy(zero[:], secret)
	return zero, keyVersion, nil
}

type storedPolicy struct {
	revision  uint64
	signature string
	digest    string
}

func (store *postgresStore) reconcilePoliciesTx(ctx context.Context, tx *sql.Tx, oldVersion uint64, oldDigest string, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `
SELECT policy_id, revision, signature, definition_digest
FROM abuse_policy_definitions
WHERE instance_key = $1
FOR UPDATE`, store.instanceKey)
	if err != nil {
		return storeError("load policy definitions", err)
	}
	existing := map[PolicyID]storedPolicy{}
	for rows.Next() {
		var id PolicyID
		var policy storedPolicy
		if err := rows.Scan(&id, &policy.revision, &policy.signature, &policy.digest); err != nil {
			_ = rows.Close()
			return storeError("scan policy definition", err)
		}
		existing[id] = policy
	}
	if err := rows.Close(); err != nil {
		return storeError("close policy definitions", err)
	}
	for id, meter := range store.policies {
		encoded, err := json.Marshal(meter.def)
		if err != nil {
			return &Error{Kind: ErrorInvalidDefinition, Field: "policy", Message: "cannot encode definition", Cause: err}
		}
		digestBytes := sha256.Sum256(encoded)
		digest := hex.EncodeToString(digestBytes[:])
		signature := policySignature(meter.def)
		if old, ok := existing[id]; ok {
			if meter.def.Revision < old.revision {
				return &Error{Kind: ErrorDefinitionDrift, Field: "policy", Message: "policy revision rollback is not allowed"}
			}
			if signature != old.signature {
				return &Error{
					Kind: ErrorDefinitionDrift, Field: "policy",
					Message: "algorithm, signal slot, or meter mode changed without a new policy id",
				}
			}
			if meter.def.Revision == old.revision && digest != old.digest {
				return &Error{Kind: ErrorDefinitionDrift, Field: "policy", Message: "same policy revision has a different digest"}
			}
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO abuse_policy_definitions (
    instance_key, policy_id, revision, signature, definition_digest, definition, active, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, TRUE, $7)
ON CONFLICT (instance_key, policy_id) DO UPDATE
SET revision = EXCLUDED.revision,
    signature = EXCLUDED.signature,
    definition_digest = EXCLUDED.definition_digest,
    definition = EXCLUDED.definition,
    active = TRUE,
    updated_at = EXCLUDED.updated_at`,
			store.instanceKey, id, meter.def.Revision, signature, digest, string(encoded), now,
		); err != nil {
			return storeError("reconcile policy definition", err)
		}
	}
	if oldVersion < store.catalog.version || oldDigest != store.catalog.digest {
		ids := make([]string, 0, len(store.policies))
		for id := range store.policies {
			ids = append(ids, string(id))
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE abuse_policy_definitions
SET active = FALSE, updated_at = $2
WHERE instance_key = $1 AND NOT (policy_id = ANY($3::text[]))`,
			store.instanceKey, now, pq.Array(ids),
		); err != nil {
			return storeError("retire policy definitions", err)
		}
	}
	return nil
}

func policySignature(def MeterDefinition) string {
	value := fmt.Sprintf("%s\x00%s\x00%s", def.Slot, def.Mode, def.Algorithm.Kind)
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (store *postgresStore) begin(ctx context.Context) (*sql.Tx, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, storeError("begin transaction", err)
	}
	timeout := fmt.Sprintf("%dms", store.lockTimeout.Milliseconds())
	if _, err := tx.ExecContext(ctx, `SELECT set_config('lock_timeout', $1, TRUE)`, timeout); err != nil {
		_ = tx.Rollback()
		return nil, storeError("set lock timeout", err)
	}
	return tx, nil
}

func (store *postgresStore) nowTx(ctx context.Context, tx *sql.Tx) (time.Time, error) {
	if store.clock != nil {
		return store.clock().UTC(), nil
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return time.Time{}, storeError("read database time", err)
	}
	return now.UTC(), nil
}

func (store *postgresStore) loadReceiptTx(ctx context.Context, tx *sql.Tx, id AttemptID) (receiptRecord, bool, error) {
	var (
		fingerprint []byte
		encoded     []byte
	)
	err := tx.QueryRowContext(ctx, `
SELECT fingerprint, record
FROM abuse_attempt_receipts
WHERE instance_key = $1 AND attempt_id = $2
FOR UPDATE`, store.instanceKey, id).Scan(&fingerprint, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return receiptRecord{}, false, nil
	}
	if err != nil {
		return receiptRecord{}, false, storeError("load receipt", err)
	}
	var record receiptRecord
	if err := json.Unmarshal(encoded, &record); err != nil {
		return receiptRecord{}, false, storeError("decode receipt", err)
	}
	if len(fingerprint) != 32 || !bytes.Equal(fingerprint, record.Fingerprint[:]) {
		return receiptRecord{}, false, &Error{Kind: ErrorStoreUnavailable, Field: "receipt", Message: "stored fingerprint is invalid"}
	}
	return record, true, nil
}

func (store *postgresStore) writeReceiptTx(ctx context.Context, tx *sql.Tx, record receiptRecord) error {
	encoded, err := json.Marshal(record)
	if err != nil {
		return storeError("encode receipt", err)
	}
	var pending any
	if !record.PendingUntil.IsZero() {
		pending = record.PendingUntil
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO abuse_attempt_receipts (
    instance_key, attempt_id, fingerprint, action_key, disposition,
    resolved_outcome, default_outcome, pending_until, record, expires_at, updated_at
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (instance_key, attempt_id) DO UPDATE
SET disposition = EXCLUDED.disposition,
    resolved_outcome = EXCLUDED.resolved_outcome,
    default_outcome = EXCLUDED.default_outcome,
    pending_until = EXCLUDED.pending_until,
    record = EXCLUDED.record,
    expires_at = EXCLUDED.expires_at,
    updated_at = EXCLUDED.updated_at`,
		store.instanceKey, record.ID, record.Fingerprint[:], record.Action, record.Disposition,
		record.ResolvedOutcome, record.DefaultOutcome, pending, string(encoded), record.ExpiresAt, record.UpdatedAt,
	)
	if err != nil {
		return storeError("write receipt", err)
	}
	return nil
}

func (store *postgresStore) lockStateTx(
	ctx context.Context,
	tx *sql.Tx,
	prepared preparedMeter,
	seedTime time.Time,
) (meterState, error) {
	empty, _ := json.Marshal(meterState{})
	if _, err := tx.ExecContext(ctx, `
INSERT INTO abuse_meter_states (
    instance_key, policy_id, subject_key_version, subject_key, state, expires_at, updated_at
)
VALUES ($1,$2,$3,$4,$5,$6,$6)
ON CONFLICT DO NOTHING`,
		store.instanceKey, prepared.key.policy, prepared.key.keyVersion,
		prepared.key.subject[:], string(empty), seedTime,
	); err != nil {
		return meterState{}, storeError("materialize meter state", err)
	}
	var encoded []byte
	if err := tx.QueryRowContext(ctx, `
SELECT state
FROM abuse_meter_states
WHERE instance_key=$1 AND policy_id=$2 AND subject_key_version=$3 AND subject_key=$4
FOR UPDATE`,
		store.instanceKey, prepared.key.policy, prepared.key.keyVersion, prepared.key.subject[:],
	).Scan(&encoded); err != nil {
		return meterState{}, storeError("lock meter state", err)
	}
	var state meterState
	if err := json.Unmarshal(encoded, &state); err != nil {
		return meterState{}, storeError("decode meter state", err)
	}
	return state, nil
}

func (store *postgresStore) writeStateTx(ctx context.Context, tx *sql.Tx, key stateKey, state meterState, now time.Time) error {
	encoded, err := json.Marshal(state)
	if err != nil {
		return storeError("encode meter state", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE abuse_meter_states
SET state=$5, expires_at=$6, updated_at=$7
WHERE instance_key=$1 AND policy_id=$2 AND subject_key_version=$3 AND subject_key=$4`,
		store.instanceKey, key.policy, key.keyVersion, key.subject[:],
		string(encoded), state.ExpiresAt, now,
	); err != nil {
		return storeError("write meter state", err)
	}
	return nil
}

func (store *postgresStore) admit(ctx context.Context, attempt preparedAttempt, proofSatisfied bool) (Admission, error) {
	tx, err := store.begin(ctx)
	if err != nil {
		return Admission{}, err
	}
	defer func() { _ = tx.Rollback() }()
	existing, exists, err := store.loadReceiptTx(ctx, tx, attempt.id)
	if err != nil {
		return Admission{}, err
	}
	if exists {
		if existing.Fingerprint != attempt.fingerprint || existing.Action != attempt.action.def.Key {
			return Admission{}, conflict("id", "is reused with a different attempt")
		}
		if existing.Disposition != DispositionChallenge || !proofSatisfied {
			return admissionFromRecord(existing, true), nil
		}
	}
	seedTime := time.Unix(0, 0).UTC()
	states := make(map[stateKey]meterState, len(attempt.meters))
	for _, prepared := range attempt.meters {
		state, err := store.lockStateTx(ctx, tx, prepared, seedTime)
		if err != nil {
			return Admission{}, err
		}
		states[prepared.key] = state
	}
	now, err := store.nowTx(ctx, tx)
	if err != nil {
		return Admission{}, err
	}
	findings := make([]Finding, 0, len(attempt.meters))
	disposition := DispositionAllow
	var retry time.Time
	for _, prepared := range attempt.meters {
		state := states[prepared.key]
		if state.Algorithm == "" {
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
	if exists {
		record.CreatedAt = existing.CreatedAt
	}
	if disposition == DispositionChallenge {
		record.Challenge = &Challenge{Kind: attempt.action.def.Challenge.Kind, ContinuationID: attempt.id}
	} else if disposition == DispositionAllow {
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
			states[prepared.key] = state
			if prepared.compiled.def.Mode == MeterOutcome {
				record.ResolutionRefs = append(record.ResolutionRefs, resolutionRef{Key: prepared.key})
			}
		}
	}
	for _, prepared := range attempt.meters {
		if err := store.writeStateTx(ctx, tx, prepared.key, states[prepared.key], now); err != nil {
			return Admission{}, err
		}
	}
	if err := store.writeReceiptTx(ctx, tx, record); err != nil {
		return Admission{}, err
	}
	if err := tx.Commit(); err != nil {
		return Admission{}, storeError("commit admission", err)
	}
	return admissionFromRecord(record, false), nil
}

func (store *postgresStore) resolve(ctx context.Context, receipt Receipt, outcome OutcomeKey) error {
	tx, err := store.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	record, exists, err := store.loadReceiptTx(ctx, tx, receipt.id)
	if err != nil {
		return err
	}
	if !exists || record.Fingerprint != receipt.fingerprint || record.Action != receipt.action {
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
	slices.SortFunc(record.ResolutionRefs, func(a, b resolutionRef) int { return a.Key.less(b.Key) })
	states := make(map[stateKey]meterState, len(record.ResolutionRefs))
	for _, ref := range record.ResolutionRefs {
		meter, ok := store.policies[ref.Key.policy]
		if !ok {
			return &Error{Kind: ErrorDefinitionDrift, Field: "policy", Message: "receipt policy is unavailable"}
		}
		prepared := preparedMeter{compiled: meter, key: ref.Key, slot: meter.def.Slot}
		state, err := store.lockStateTx(ctx, tx, prepared, time.Unix(0, 0).UTC())
		if err != nil {
			return err
		}
		states[ref.Key] = state
	}
	now, err := store.nowTx(ctx, tx)
	if err != nil {
		return err
	}
	for _, ref := range record.ResolutionRefs {
		meter := store.policies[ref.Key.policy]
		state := states[ref.Key]
		resolveState(&state, meter, record.ID, outcome, now)
		if err := store.writeStateTx(ctx, tx, ref.Key, state, now); err != nil {
			return err
		}
	}
	record.ResolvedOutcome = outcome
	record.UpdatedAt = now
	if err := store.writeReceiptTx(ctx, tx, record); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return storeError("commit resolution", err)
	}
	return nil
}

func (store *postgresStore) inspect(ctx context.Context, attempt preparedAttempt) (Inspection, error) {
	tx, err := store.begin(ctx)
	if err != nil {
		return Inspection{}, err
	}
	defer func() { _ = tx.Rollback() }()
	states := make(map[stateKey]meterState, len(attempt.meters))
	for _, prepared := range attempt.meters {
		state, err := store.lockStateTx(ctx, tx, prepared, time.Unix(0, 0).UTC())
		if err != nil {
			return Inspection{}, err
		}
		states[prepared.key] = state
	}
	now, err := store.nowTx(ctx, tx)
	if err != nil {
		return Inspection{}, err
	}
	result := Inspection{Action: attempt.action.def.Key}
	for _, prepared := range attempt.meters {
		state := states[prepared.key]
		if state.Algorithm == "" {
			state = newMeterState(prepared.compiled, now)
		}
		settleState(&state, prepared.compiled, now)
		if err := store.writeStateTx(ctx, tx, prepared.key, state, now); err != nil {
			return Inspection{}, err
		}
		result.Meters = append(result.Meters, MeterSnapshot{
			Policy: prepared.compiled.def.ID, Slot: prepared.slot,
			SubjectRef: prepared.key.subject.hex(), Used: stateUsage(state, prepared.compiled),
			Capacity: prepared.compiled.def.Algorithm.Capacity, ExpiresAt: state.ExpiresAt,
		})
	}
	if err := tx.Commit(); err != nil {
		return Inspection{}, storeError("commit inspection", err)
	}
	return result, nil
}

func (store *postgresStore) reset(ctx context.Context, attempt preparedAttempt) (ResetResult, error) {
	tx, err := store.begin(ctx)
	if err != nil {
		return ResetResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	states := make(map[stateKey]meterState, len(attempt.meters))
	for _, prepared := range attempt.meters {
		state, err := store.lockStateTx(ctx, tx, prepared, time.Unix(0, 0).UTC())
		if err != nil {
			return ResetResult{}, err
		}
		states[prepared.key] = state
	}
	now, err := store.nowTx(ctx, tx)
	if err != nil {
		return ResetResult{}, err
	}
	var result ResetResult
	for _, prepared := range attempt.meters {
		state := states[prepared.key]
		if state.Algorithm != "" {
			result.MetersReset++
			result.EventsRemoved += int64(len(state.Events))
		}
		state = newMeterState(prepared.compiled, now)
		if err := store.writeStateTx(ctx, tx, prepared.key, state, now); err != nil {
			return ResetResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ResetResult{}, storeError("commit reset", err)
	}
	return result, nil
}

func (store *postgresStore) prune(ctx context.Context, command PruneCommand) (PruneResult, error) {
	var result PruneResult
	rows, err := store.db.QueryContext(ctx, `
SELECT attempt_id, action_key, fingerprint, default_outcome
FROM abuse_attempt_receipts
WHERE instance_key=$1 AND disposition='allow' AND resolved_outcome=''
  AND default_outcome<>'' AND pending_until <= $2
ORDER BY pending_until, attempt_id
LIMIT $3`, store.instanceKey, store.currentTime(), command.Limit)
	if err != nil {
		return result, storeError("query pending receipts", err)
	}
	type pendingReceipt struct {
		id          AttemptID
		action      ActionKey
		fingerprint [32]byte
		outcome     OutcomeKey
	}
	var pending []pendingReceipt
	for rows.Next() {
		var item pendingReceipt
		var fingerprint []byte
		if err := rows.Scan(&item.id, &item.action, &fingerprint, &item.outcome); err != nil {
			_ = rows.Close()
			return result, storeError("scan pending receipt", err)
		}
		if len(fingerprint) != 32 {
			_ = rows.Close()
			return result, &Error{Kind: ErrorStoreUnavailable, Field: "receipt", Message: "stored fingerprint is invalid"}
		}
		copy(item.fingerprint[:], fingerprint)
		pending = append(pending, item)
	}
	if err := rows.Close(); err != nil {
		return result, storeError("close pending receipts", err)
	}
	for _, item := range pending {
		if err := store.resolve(ctx, Receipt{
			id: item.id, action: item.action, fingerprint: item.fingerprint,
		}, item.outcome); err != nil {
			return result, err
		}
		result.PendingFinalized++
	}

	tx, err := store.begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	stateRows, err := tx.QueryContext(ctx, `
SELECT policy_id, subject_key_version, subject_key, state
FROM abuse_meter_states
WHERE instance_key=$1
ORDER BY policy_id, subject_key_version, subject_key
LIMIT $2
FOR UPDATE`, store.instanceKey, command.Limit)
	if err != nil {
		return result, storeError("query meter states for prune", err)
	}
	type lockedState struct {
		key   stateKey
		state meterState
	}
	var locked []lockedState
	for stateRows.Next() {
		var item lockedState
		var subject, encoded []byte
		if err := stateRows.Scan(&item.key.policy, &item.key.keyVersion, &subject, &encoded); err != nil {
			_ = stateRows.Close()
			return result, storeError("scan meter state for prune", err)
		}
		if len(subject) != 32 {
			_ = stateRows.Close()
			return result, &Error{Kind: ErrorStoreUnavailable, Field: "subject_key", Message: "stored value is invalid"}
		}
		copy(item.key.subject[:], subject)
		if err := json.Unmarshal(encoded, &item.state); err != nil {
			_ = stateRows.Close()
			return result, storeError("decode meter state for prune", err)
		}
		locked = append(locked, item)
	}
	if err := stateRows.Close(); err != nil {
		return result, storeError("close meter states for prune", err)
	}
	now, err := store.nowTx(ctx, tx)
	if err != nil {
		return result, err
	}
	for _, item := range locked {
		meter, active := store.policies[item.key.policy]
		if !active {
			if item.state.ExpiresAt.Before(command.Before) {
				if _, err := tx.ExecContext(ctx, `
DELETE FROM abuse_meter_states
WHERE instance_key=$1 AND policy_id=$2 AND subject_key_version=$3 AND subject_key=$4`,
					store.instanceKey, item.key.policy, item.key.keyVersion, item.key.subject[:],
				); err != nil {
					return result, storeError("delete retired meter state", err)
				}
				result.EventsRemoved += int64(len(item.state.Events))
				result.StatesRemoved++
			}
			continue
		}
		removed, finalized := settleState(&item.state, meter, now)
		result.EventsRemoved += removed
		result.PendingFinalized += finalized
		if item.state.ExpiresAt.Before(command.Before) && stateUsage(item.state, meter) == 0 {
			if _, err := tx.ExecContext(ctx, `
DELETE FROM abuse_meter_states
WHERE instance_key=$1 AND policy_id=$2 AND subject_key_version=$3 AND subject_key=$4`,
				store.instanceKey, item.key.policy, item.key.keyVersion, item.key.subject[:],
			); err != nil {
				return result, storeError("delete expired meter state", err)
			}
			result.StatesRemoved++
		} else if err := store.writeStateTx(ctx, tx, item.key, item.state, now); err != nil {
			return result, err
		}
	}
	deleted, err := tx.ExecContext(ctx, `
DELETE FROM abuse_attempt_receipts
WHERE instance_key=$1 AND expires_at < $2
  AND (disposition <> 'allow' OR resolved_outcome <> '' OR default_outcome = '')`,
		store.instanceKey, command.Before,
	)
	if err != nil {
		return result, storeError("delete expired receipts", err)
	}
	result.ReceiptsRemoved, _ = deleted.RowsAffected()
	if err := tx.Commit(); err != nil {
		return result, storeError("commit prune", err)
	}
	return result, nil
}

func (store *postgresStore) currentTime() time.Time {
	if store.clock != nil {
		return store.clock().UTC()
	}
	return time.Now().UTC()
}

func storeError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var pqError *pq.Error
	if errors.As(err, &pqError) {
		switch string(pqError.Code) {
		case "55P03", "40P01", "40001":
			return &Error{
				Kind: ErrorStoreContention, Field: "postgres",
				Message: operation + " encountered contention", Retryable: true, Cause: err,
			}
		}
	}
	return &Error{
		Kind: ErrorStoreUnavailable, Field: "postgres",
		Message: operation + " failed", Retryable: true, Cause: err,
	}
}
