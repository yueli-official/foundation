package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yueli-official/foundation/go/work"
)

type Options struct {
	DB          *sql.DB
	InstanceKey string
	Clock       func() time.Time
}

type Adapter struct {
	db          *sql.DB
	instanceKey string
	catalog     *work.Catalog
	clock       func() time.Time
	created     bool
}

var _ work.Backend = (*Adapter)(nil)

func New(ctx context.Context, catalog *work.Catalog, options Options) (*Adapter, error) {
	if catalog == nil {
		return nil, typedInvalid("catalog", "is required")
	}
	if options.DB == nil {
		return nil, typedInvalid("db", "is required")
	}
	instanceKey := strings.TrimSpace(options.InstanceKey)
	if instanceKey == "" {
		return nil, typedInvalid("instance_key", "is required")
	}
	if len(instanceKey) > 200 || strings.ContainsRune(instanceKey, '\x00') {
		return nil, typedInvalid("instance_key", "is invalid")
	}
	if err := options.DB.PingContext(ctx); err != nil {
		return nil, unavailable("ping database", err)
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	now := clock().UTC()
	tx, err := options.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, unavailable("begin instance bootstrap", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
INSERT INTO work_instances (
    instance_key, schema_version, catalog_version, catalog_digest, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (instance_key) DO NOTHING`,
		instanceKey, CurrentSchemaVersion, catalog.Version(), catalog.Digest(), now)
	if err != nil {
		return nil, unavailable("bootstrap instance", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, unavailable("read bootstrap result", err)
	}
	var schemaVersion, catalogVersion int64
	var digest string
	if err := tx.QueryRowContext(ctx, `
SELECT schema_version, catalog_version, catalog_digest
FROM work_instances
WHERE instance_key = $1
FOR UPDATE`, instanceKey).Scan(&schemaVersion, &catalogVersion, &digest); err != nil {
		return nil, unavailable("load instance", err)
	}
	if schemaVersion != int64(CurrentSchemaVersion) {
		return nil, &work.Error{
			Kind: work.ErrorUnavailable, Field: "schema_version",
			Message: fmt.Sprintf("database has %d, module requires %d", schemaVersion, CurrentSchemaVersion),
		}
	}
	if catalogVersion != int64(catalog.Version()) || digest != catalog.Digest() {
		return nil, &work.Error{
			Kind: work.ErrorConflict, Field: "catalog",
			Message: "database catalog version or digest does not match the compiled definition",
		}
	}
	for _, key := range catalog.ScheduleKeys() {
		next, _ := catalog.NextSchedule(key, now)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO work_schedules (instance_key, schedule_key, next_run_at, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (instance_key, schedule_key) DO NOTHING`,
			instanceKey, key, next, now); err != nil {
			return nil, unavailable("bootstrap schedule", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, unavailable("commit instance bootstrap", err)
	}
	return &Adapter{
		db: options.DB, instanceKey: instanceKey, catalog: catalog, clock: clock,
		created: affected == 1,
	}, nil
}

func (adapter *Adapter) InstanceWasCreated() bool {
	return adapter != nil && adapter.created
}

func (adapter *Adapter) Enqueue(ctx context.Context, request work.Request) (work.EnqueueResult, error) {
	if err := ctx.Err(); err != nil {
		return work.EnqueueResult{}, err
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return work.EnqueueResult{}, unavailable("begin enqueue", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := adapter.enqueueTx(ctx, tx, request, "")
	if err != nil {
		return work.EnqueueResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return work.EnqueueResult{}, unavailable("commit enqueue", err)
	}
	return result, nil
}

// EnqueueTx atomically writes a job with caller-owned domain data. The caller
// owns commit and rollback. GoFrame consumers can pass gdb.TX.GetSqlTX().
func (adapter *Adapter) EnqueueTx(
	ctx context.Context,
	tx *sql.Tx,
	request work.Request,
) (work.EnqueueResult, error) {
	if tx == nil {
		return work.EnqueueResult{}, typedInvalid("tx", "is required")
	}
	return adapter.enqueueTx(ctx, tx, request, "")
}

func (adapter *Adapter) enqueueTx(
	ctx context.Context,
	tx *sql.Tx,
	request work.Request,
	replayOf work.JobID,
) (work.EnqueueResult, error) {
	now := adapter.clock().UTC()
	prepared, err := adapter.catalog.Prepare(now, request)
	if err != nil {
		return work.EnqueueResult{}, err
	}
	if replayOf != "" {
		prepared, err = adapter.catalog.PrepareReplay(prepared, replayOf)
		if err != nil {
			return work.EnqueueResult{}, err
		}
	}
	return adapter.enqueuePreparedTx(ctx, tx, now, prepared, replayOf)
}

func (adapter *Adapter) enqueuePreparedTx(
	ctx context.Context,
	tx *sql.Tx,
	now time.Time,
	prepared work.PreparedRequest,
	replayOf work.JobID,
) (work.EnqueueResult, error) {
	id, err := work.NewJobID()
	if err != nil {
		return work.EnqueueResult{}, err
	}
	var replayValue any
	if replayOf != "" {
		replayValue = replayOf
	}
	row := tx.QueryRowContext(ctx, `
INSERT INTO work_jobs (
    instance_key, job_id, kind, queue, status, payload, metadata,
    attempt, max_attempts, priority, run_at, fingerprint, idempotency_key,
    replay_of, created_at, updated_at
)
VALUES (
    $1, $2, $3, $4, 'queued', $5::jsonb, $6::jsonb,
    0, $7, $8, $9, $10, $11, $12, $13, $13
)
ON CONFLICT (instance_key, idempotency_key)
    WHERE idempotency_key <> ''
DO NOTHING
RETURNING `+jobColumns,
		adapter.instanceKey, id, prepared.Kind, prepared.Queue,
		string(prepared.Payload), string(prepared.Metadata), prepared.MaxAttempts,
		prepared.Priority, prepared.RunAt, prepared.Fingerprint[:],
		prepared.IdempotencyKey, replayValue, now,
	)
	job, err := scanJob(row)
	if err == nil {
		return work.EnqueueResult{Job: job}, nil
	}
	if err != sql.ErrNoRows {
		return work.EnqueueResult{}, unavailable("insert job", err)
	}
	var storedFingerprint []byte
	row = tx.QueryRowContext(ctx, `
SELECT `+jobColumns+`, fingerprint
FROM work_jobs
WHERE instance_key = $1 AND idempotency_key = $2`,
		adapter.instanceKey, prepared.IdempotencyKey)
	job, err = scanJobWithFingerprint(row, &storedFingerprint)
	if err != nil {
		return work.EnqueueResult{}, unavailable("load idempotent job", err)
	}
	if !bytes.Equal(storedFingerprint, prepared.Fingerprint[:]) {
		return work.EnqueueResult{}, typedConflict(
			"idempotency_key",
			fmt.Sprintf("%q is reused with a different request", prepared.IdempotencyKey),
		)
	}
	return work.EnqueueResult{Job: job, Replay: true}, nil
}

const jobColumns = `
job_id, kind, queue, status, payload, metadata, progress, result,
result_summary, last_error, attempt, max_attempts, priority, run_at,
created_at, updated_at, started_at, finished_at, cancelled_at, replay_of,
idempotency_key, lease_owner, lease_expires_at`

type scanner interface {
	Scan(...any) error
}

func scanJob(row scanner) (work.Job, error) {
	return scanJobWithFingerprint(row, nil)
}

func scanJobWithFingerprint(row scanner, fingerprint *[]byte) (work.Job, error) {
	var (
		job                                        work.Job
		payload, metadata                          []byte
		progress, result                           []byte
		started, finished, cancelled, leaseExpires sql.NullTime
		replay                                     sql.NullString
	)
	destinations := []any{
		&job.ID, &job.Kind, &job.Queue, &job.Status, &payload, &metadata,
		&progress, &result, &job.ResultSummary, &job.LastError, &job.Attempt,
		&job.MaxAttempts, &job.Priority, &job.RunAt, &job.CreatedAt, &job.UpdatedAt,
		&started, &finished, &cancelled, &replay, &job.IdempotencyKey,
		&job.LeaseOwner, &leaseExpires,
	}
	if fingerprint != nil {
		destinations = append(destinations, fingerprint)
	}
	if err := row.Scan(destinations...); err != nil {
		return work.Job{}, err
	}
	job.Payload = compactJSON(payload)
	job.Metadata = compactJSON(metadata)
	job.Progress = compactJSON(progress)
	job.Result = compactJSON(result)
	job.RunAt = job.RunAt.UTC()
	job.CreatedAt = job.CreatedAt.UTC()
	job.UpdatedAt = job.UpdatedAt.UTC()
	if started.Valid {
		job.StartedAt = timePointer(started.Time.UTC())
	}
	if finished.Valid {
		job.FinishedAt = timePointer(finished.Time.UTC())
	}
	if cancelled.Valid {
		job.CancelledAt = timePointer(cancelled.Time.UTC())
	}
	if replay.Valid {
		job.ReplayOf = work.JobID(replay.String)
	}
	if leaseExpires.Valid {
		job.LeaseExpiresAt = timePointer(leaseExpires.Time.UTC())
	}
	return job, nil
}

func compactJSON(value []byte) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	var output bytes.Buffer
	if err := json.Compact(&output, value); err != nil {
		return append(json.RawMessage(nil), value...)
	}
	return append(json.RawMessage(nil), output.Bytes()...)
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func typedInvalid(field, message string) error {
	return &work.Error{Kind: work.ErrorInvalidInput, Field: field, Message: message}
}

func typedConflict(field, message string) error {
	return &work.Error{Kind: work.ErrorConflict, Field: field, Message: message}
}

func unavailable(operation string, err error) error {
	return &work.Error{
		Kind: work.ErrorUnavailable, Field: "postgres",
		Message: operation + " failed", Cause: err,
	}
}
