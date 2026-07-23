package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/yueli-official/foundation/go/work"
)

func (adapter *Adapter) Pause(ctx context.Context, id work.JobID) (work.Job, error) {
	return adapter.transition(ctx, id, "pause")
}

func (adapter *Adapter) Resume(ctx context.Context, id work.JobID) (work.Job, error) {
	return adapter.transition(ctx, id, "resume")
}

func (adapter *Adapter) Cancel(ctx context.Context, id work.JobID) (work.Job, error) {
	return adapter.transition(ctx, id, "cancel")
}

func (adapter *Adapter) transition(ctx context.Context, id work.JobID, operation string) (work.Job, error) {
	if err := ctx.Err(); err != nil {
		return work.Job{}, err
	}
	now := adapter.clock().UTC()
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return work.Job{}, unavailable("begin state transition", err)
	}
	defer func() { _ = tx.Rollback() }()
	job, err := scanJob(tx.QueryRowContext(ctx, `
SELECT `+jobColumns+`
FROM work_jobs
WHERE instance_key = $1 AND job_id = $2
FOR UPDATE`, adapter.instanceKey, id))
	if err == sql.ErrNoRows {
		return work.Job{}, &work.Error{
			Kind: work.ErrorNotFound, Field: "job_id",
			Message: fmt.Sprintf("%q does not exist", id),
		}
	}
	if err != nil {
		return work.Job{}, unavailable("lock job", err)
	}
	next := job.Status
	pausedFrom := ""
	cancelledAt := any(nil)
	finishedAt := any(nil)
	attemptOutcome := work.AttemptOutcome("")
	switch operation {
	case "pause":
		switch job.Status {
		case work.StatusQueued, work.StatusRetrying, work.StatusRunning:
			next = work.StatusPaused
			pausedFrom = string(job.Status)
			if job.Status == work.StatusRunning {
				attemptOutcome = work.AttemptPaused
			}
		default:
			return work.Job{}, typedConflict("status", fmt.Sprintf("cannot pause %s job", job.Status))
		}
	case "resume":
		if job.Status != work.StatusPaused {
			return work.Job{}, typedConflict("status", fmt.Sprintf("cannot resume %s job", job.Status))
		}
		if job.Attempt > 0 {
			next = work.StatusRetrying
		} else {
			next = work.StatusQueued
		}
	case "cancel":
		switch job.Status {
		case work.StatusQueued, work.StatusRetrying, work.StatusPaused, work.StatusRunning:
			next = work.StatusCancelled
			cancelledAt = now
			finishedAt = now
			if job.Status == work.StatusRunning {
				attemptOutcome = work.AttemptCancelled
			}
		default:
			return work.Job{}, typedConflict("status", fmt.Sprintf("cannot cancel %s job", job.Status))
		}
	}
	if attemptOutcome != "" {
		if _, err := tx.ExecContext(ctx, `
UPDATE work_attempts
SET outcome = $4, finished_at = $5
WHERE instance_key = $1 AND job_id = $2 AND attempt_number = $3 AND outcome = 'running'`,
			adapter.instanceKey, id, job.Attempt, attemptOutcome, now); err != nil {
			return work.Job{}, unavailable("finish interrupted attempt", err)
		}
	}
	job, err = scanJob(tx.QueryRowContext(ctx, `
UPDATE work_jobs
SET status = $3,
    paused_from = $4,
    lease_token = NULL,
    lease_owner = '',
    lease_expires_at = NULL,
    cancelled_at = $5,
    finished_at = $6,
    updated_at = $7
WHERE instance_key = $1 AND job_id = $2
RETURNING `+jobColumns,
		adapter.instanceKey, id, next, pausedFrom, cancelledAt, finishedAt, now))
	if err != nil {
		return work.Job{}, unavailable("update job state", err)
	}
	if err := tx.Commit(); err != nil {
		return work.Job{}, unavailable("commit state transition", err)
	}
	return job, nil
}

func (adapter *Adapter) Replay(
	ctx context.Context,
	id work.JobID,
	command work.ReplayRequest,
) (work.EnqueueResult, error) {
	if err := ctx.Err(); err != nil {
		return work.EnqueueResult{}, err
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return work.EnqueueResult{}, unavailable("begin replay", err)
	}
	defer func() { _ = tx.Rollback() }()
	source, err := scanJob(tx.QueryRowContext(ctx, `
SELECT `+jobColumns+`
FROM work_jobs
WHERE instance_key = $1 AND job_id = $2
FOR SHARE`, adapter.instanceKey, id))
	if err == sql.ErrNoRows {
		return work.EnqueueResult{}, &work.Error{
			Kind: work.ErrorNotFound, Field: "job_id",
			Message: fmt.Sprintf("%q does not exist", id),
		}
	}
	if err != nil {
		return work.EnqueueResult{}, unavailable("load replay source", err)
	}
	if source.Status != work.StatusSucceeded && source.Status != work.StatusFailed &&
		source.Status != work.StatusCancelled {
		return work.EnqueueResult{}, typedConflict("status", "only terminal jobs can be replayed")
	}
	priority := source.Priority
	if command.Priority != nil {
		priority = *command.Priority
	}
	attempts := command.MaxAttempts
	if attempts == 0 {
		attempts = source.MaxAttempts
	}
	result, err := adapter.enqueueTx(ctx, tx, work.Request{
		Kind: source.Kind, Payload: source.Payload, Metadata: source.Metadata,
		RunAt: command.RunAt, Priority: priority, MaxAttempts: attempts,
		IdempotencyKey: command.IdempotencyKey,
	}, id)
	if err != nil {
		return work.EnqueueResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return work.EnqueueResult{}, unavailable("commit replay", err)
	}
	return result, nil
}

func (adapter *Adapter) Prune(ctx context.Context, before time.Time) (work.PruneResult, error) {
	if err := ctx.Err(); err != nil {
		return work.PruneResult{}, err
	}
	if before.IsZero() {
		return work.PruneResult{}, typedInvalid("before", "is required")
	}
	before = before.UTC()
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return work.PruneResult{}, unavailable("begin prune", err)
	}
	defer func() { _ = tx.Rollback() }()
	var result work.PruneResult
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM work_attempts AS attempt
JOIN work_jobs AS job
  ON job.instance_key = attempt.instance_key AND job.job_id = attempt.job_id
WHERE job.instance_key = $1
  AND job.status IN ('succeeded', 'failed', 'cancelled')
  AND job.finished_at < $2`, adapter.instanceKey, before).Scan(&result.AttemptsRemoved); err != nil {
		return work.PruneResult{}, unavailable("count pruned attempts", err)
	}
	sqlResult, err := tx.ExecContext(ctx, `
DELETE FROM work_jobs
WHERE instance_key = $1
  AND status IN ('succeeded', 'failed', 'cancelled')
  AND finished_at < $2`, adapter.instanceKey, before)
	if err != nil {
		return work.PruneResult{}, unavailable("delete terminal jobs", err)
	}
	result.JobsRemoved, err = sqlResult.RowsAffected()
	if err != nil {
		return work.PruneResult{}, unavailable("read prune result", err)
	}
	if err := tx.Commit(); err != nil {
		return work.PruneResult{}, unavailable("commit prune", err)
	}
	return result, nil
}
