package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/yueli-official/foundation/go/work"
)

func (adapter *Adapter) Claim(ctx context.Context, command work.ClaimRequest) (work.ClaimResult, error) {
	if err := ctx.Err(); err != nil {
		return work.ClaimResult{}, err
	}
	registered := false
	for _, queue := range adapter.catalog.Queues() {
		if queue.Key == command.Queue {
			registered = true
			break
		}
	}
	if !registered {
		return work.ClaimResult{}, typedInvalid("queue", "is not registered")
	}
	workerID := strings.TrimSpace(command.WorkerID)
	if workerID == "" || len(workerID) > 512 {
		return work.ClaimResult{}, typedInvalid("worker_id", "is invalid")
	}
	if err := adapter.catalog.ValidateLease(command.LeaseDuration); err != nil {
		return work.ClaimResult{}, err
	}
	now := adapter.clock().UTC()
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return work.ClaimResult{}, unavailable("begin claim", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := adapter.reclaimExpiredTx(ctx, tx, now); err != nil {
		return work.ClaimResult{}, err
	}
	token, err := work.NewJobID()
	if err != nil {
		return work.ClaimResult{}, err
	}
	expires := now.Add(command.LeaseDuration)
	job, err := scanJob(tx.QueryRowContext(ctx, `
WITH candidate AS (
    SELECT job_id
    FROM work_jobs
    WHERE instance_key = $1
      AND queue = $2
      AND status IN ('queued', 'retrying')
      AND run_at <= $3
      AND attempt < max_attempts
    ORDER BY priority DESC, run_at, created_at, job_id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE work_jobs AS job
SET status = 'running',
    attempt = job.attempt + 1,
    lease_token = $4,
    lease_owner = $5,
    lease_expires_at = $6,
    started_at = COALESCE(job.started_at, $3),
    updated_at = $3
FROM candidate
WHERE job.instance_key = $1 AND job.job_id = candidate.job_id
RETURNING `+prefixedJobColumns("job"),
		adapter.instanceKey, command.Queue, now, token, workerID, expires))
	if err == sql.ErrNoRows {
		if err := tx.Commit(); err != nil {
			return work.ClaimResult{}, unavailable("commit empty claim", err)
		}
		return work.ClaimResult{}, nil
	}
	if err != nil {
		return work.ClaimResult{}, unavailable("claim job", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO work_attempts (
    instance_key, job_id, attempt_number, worker_id, outcome, started_at
)
VALUES ($1, $2, $3, $4, 'running', $5)`,
		adapter.instanceKey, job.ID, job.Attempt, workerID, now); err != nil {
		return work.ClaimResult{}, unavailable("create attempt", err)
	}
	if err := tx.Commit(); err != nil {
		return work.ClaimResult{}, unavailable("commit claim", err)
	}
	return work.ClaimResult{Found: true, Lease: work.Lease{
		Job: job, Token: string(token), ExpiresAt: expires,
	}}, nil
}

func (adapter *Adapter) reclaimExpiredTx(ctx context.Context, tx *sql.Tx, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `
SELECT job_id, attempt, max_attempts
FROM work_jobs
WHERE instance_key = $1
  AND status = 'running'
  AND lease_expires_at <= $2
ORDER BY lease_expires_at
FOR UPDATE SKIP LOCKED
LIMIT 100`, adapter.instanceKey, now)
	if err != nil {
		return unavailable("find expired leases", err)
	}
	type expired struct {
		id          work.JobID
		attempt     int
		maxAttempts int
	}
	items := make([]expired, 0)
	for rows.Next() {
		var item expired
		if err := rows.Scan(&item.id, &item.attempt, &item.maxAttempts); err != nil {
			_ = rows.Close()
			return unavailable("scan expired lease", err)
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return unavailable("close expired leases", err)
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `
UPDATE work_attempts
SET outcome = 'lease_expired', error = 'lease expired', finished_at = $4
WHERE instance_key = $1 AND job_id = $2 AND attempt_number = $3 AND outcome = 'running'`,
			adapter.instanceKey, item.id, item.attempt, now); err != nil {
			return unavailable("finish expired attempt", err)
		}
		if item.attempt >= item.maxAttempts {
			if _, err := tx.ExecContext(ctx, `
UPDATE work_jobs
SET status = 'failed', last_error = 'lease expired',
    lease_token = NULL, lease_owner = '', lease_expires_at = NULL,
    finished_at = $3, updated_at = $3
WHERE instance_key = $1 AND job_id = $2`,
				adapter.instanceKey, item.id, now); err != nil {
				return unavailable("fail expired job", err)
			}
			continue
		}
		runAt := now.Add(adapter.catalog.Backoff(item.id, item.attempt))
		if _, err := tx.ExecContext(ctx, `
UPDATE work_jobs
SET status = 'retrying', last_error = 'lease expired', run_at = $3,
    lease_token = NULL, lease_owner = '', lease_expires_at = NULL,
    updated_at = $4
WHERE instance_key = $1 AND job_id = $2`,
			adapter.instanceKey, item.id, runAt, now); err != nil {
			return unavailable("retry expired job", err)
		}
	}
	return nil
}

func (adapter *Adapter) Heartbeat(
	ctx context.Context,
	lease work.Lease,
	duration time.Duration,
) (work.Lease, error) {
	if err := ctx.Err(); err != nil {
		return work.Lease{}, err
	}
	if err := adapter.catalog.ValidateLease(duration); err != nil {
		return work.Lease{}, err
	}
	now := adapter.clock().UTC()
	expires := now.Add(duration)
	job, err := scanJob(adapter.db.QueryRowContext(ctx, `
UPDATE work_jobs
SET lease_expires_at = $4, updated_at = $5
WHERE instance_key = $1
  AND job_id = $2
  AND status = 'running'
  AND lease_token = $3
  AND lease_expires_at > $5
RETURNING `+jobColumns,
		adapter.instanceKey, lease.Job.ID, lease.Token, expires, now))
	if err == sql.ErrNoRows {
		return work.Lease{}, &work.Error{
			Kind: work.ErrorLeaseLost, Field: "lease", Message: "ownership changed or expired",
		}
	}
	if err != nil {
		return work.Lease{}, unavailable("heartbeat", err)
	}
	return work.Lease{Job: job, Token: lease.Token, ExpiresAt: expires}, nil
}

func (adapter *Adapter) ReportProgress(
	ctx context.Context,
	lease work.Lease,
	progress json.RawMessage,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	normalized, err := adapter.catalog.PrepareProgress(progress)
	if err != nil {
		return err
	}
	now := adapter.clock().UTC()
	result, err := adapter.db.ExecContext(ctx, `
UPDATE work_jobs
SET progress = $4::jsonb, updated_at = $5
WHERE instance_key = $1
  AND job_id = $2
  AND status = 'running'
  AND lease_token = $3
  AND lease_expires_at > $5`,
		adapter.instanceKey, lease.Job.ID, lease.Token, string(normalized), now)
	if err != nil {
		return unavailable("report progress", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return unavailable("read progress result", err)
	}
	if affected == 0 {
		return &work.Error{
			Kind: work.ErrorLeaseLost, Field: "lease", Message: "ownership changed or expired",
		}
	}
	return nil
}

func (adapter *Adapter) Complete(
	ctx context.Context,
	command work.CompleteCommand,
) (work.Job, error) {
	result, err := adapter.catalog.PrepareResult(command.Result)
	if err != nil {
		return work.Job{}, err
	}
	return adapter.finish(ctx, command.Lease, work.AttemptSucceeded, "", func(
		ctx context.Context, tx *sql.Tx, now time.Time,
	) (work.Job, error) {
		return scanJob(tx.QueryRowContext(ctx, `
UPDATE work_jobs
SET status = 'succeeded', result = $3::jsonb, result_summary = $4,
    last_error = '', lease_token = NULL, lease_owner = '', lease_expires_at = NULL,
    finished_at = $5, updated_at = $5
WHERE instance_key = $1 AND job_id = $2
RETURNING `+jobColumns,
			adapter.instanceKey, command.Lease.Job.ID, string(result.Data), result.Summary, now))
	})
}

func (adapter *Adapter) Retry(ctx context.Context, command work.RetryCommand) (work.Job, error) {
	if strings.TrimSpace(command.Error) == "" {
		return work.Job{}, typedInvalid("error", "is required")
	}
	now := adapter.clock().UTC()
	runAt := command.RunAt.UTC()
	if command.RunAt.IsZero() || runAt.Before(now) {
		runAt = now
	}
	if runAt.After(now.Add(adapter.catalog.Limits().MaxDelay)) {
		return work.Job{}, typedInvalid("run_at", "exceeds maximum delay")
	}
	if command.Lease.Job.Attempt >= command.Lease.Job.MaxAttempts {
		return work.Job{}, typedConflict("attempt", "attempt budget is exhausted")
	}
	return adapter.finish(ctx, command.Lease, work.AttemptRetrying, command.Error, func(
		ctx context.Context, tx *sql.Tx, now time.Time,
	) (work.Job, error) {
		return scanJob(tx.QueryRowContext(ctx, `
UPDATE work_jobs
SET status = 'retrying', last_error = $3, run_at = $4,
    lease_token = NULL, lease_owner = '', lease_expires_at = NULL,
    updated_at = $5
WHERE instance_key = $1 AND job_id = $2
RETURNING `+jobColumns,
			adapter.instanceKey, command.Lease.Job.ID, command.Error, runAt, now))
	})
}

func (adapter *Adapter) Fail(ctx context.Context, command work.FailCommand) (work.Job, error) {
	if strings.TrimSpace(command.Error) == "" {
		return work.Job{}, typedInvalid("error", "is required")
	}
	return adapter.finish(ctx, command.Lease, work.AttemptFailed, command.Error, func(
		ctx context.Context, tx *sql.Tx, now time.Time,
	) (work.Job, error) {
		return scanJob(tx.QueryRowContext(ctx, `
UPDATE work_jobs
SET status = 'failed', last_error = $3,
    lease_token = NULL, lease_owner = '', lease_expires_at = NULL,
    finished_at = $4, updated_at = $4
WHERE instance_key = $1 AND job_id = $2
RETURNING `+jobColumns,
			adapter.instanceKey, command.Lease.Job.ID, command.Error, now))
	})
}

func (adapter *Adapter) finish(
	ctx context.Context,
	lease work.Lease,
	outcome work.AttemptOutcome,
	message string,
	update func(context.Context, *sql.Tx, time.Time) (work.Job, error),
) (work.Job, error) {
	if err := ctx.Err(); err != nil {
		return work.Job{}, err
	}
	now := adapter.clock().UTC()
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return work.Job{}, unavailable("begin lease transition", err)
	}
	defer func() { _ = tx.Rollback() }()
	var attempt, maxAttempts int
	err = tx.QueryRowContext(ctx, `
SELECT attempt, max_attempts
FROM work_jobs
WHERE instance_key = $1
  AND job_id = $2
  AND status = 'running'
  AND lease_token = $3
  AND lease_expires_at > $4
FOR UPDATE`, adapter.instanceKey, lease.Job.ID, lease.Token, now).Scan(&attempt, &maxAttempts)
	if err == sql.ErrNoRows {
		return work.Job{}, &work.Error{
			Kind: work.ErrorLeaseLost, Field: "lease", Message: "ownership changed or expired",
		}
	}
	if err != nil {
		return work.Job{}, unavailable("lock leased job", err)
	}
	if outcome == work.AttemptRetrying && attempt >= maxAttempts {
		return work.Job{}, typedConflict("attempt", "attempt budget is exhausted")
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE work_attempts
SET outcome = $4, error = $5, finished_at = $6
WHERE instance_key = $1 AND job_id = $2 AND attempt_number = $3 AND outcome = 'running'`,
		adapter.instanceKey, lease.Job.ID, attempt, outcome, message, now); err != nil {
		return work.Job{}, unavailable("finish attempt", err)
	}
	job, err := update(ctx, tx, now)
	if err != nil {
		return work.Job{}, unavailable("finish job", err)
	}
	if err := tx.Commit(); err != nil {
		return work.Job{}, unavailable("commit lease transition", err)
	}
	return job, nil
}

func prefixedJobColumns(alias string) string {
	columns := strings.Split(jobColumns, ",")
	for index, column := range columns {
		columns[index] = alias + "." + strings.TrimSpace(column)
	}
	return strings.Join(columns, ", ")
}
