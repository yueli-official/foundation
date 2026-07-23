package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"github.com/yueli-official/foundation/go/work"
)

func (adapter *Adapter) Get(ctx context.Context, id work.JobID) (work.Job, error) {
	if err := ctx.Err(); err != nil {
		return work.Job{}, err
	}
	job, err := scanJob(adapter.db.QueryRowContext(ctx, `
SELECT `+jobColumns+`
FROM work_jobs
WHERE instance_key = $1 AND job_id = $2`,
		adapter.instanceKey, id))
	if err == sql.ErrNoRows {
		return work.Job{}, &work.Error{
			Kind: work.ErrorNotFound, Field: "job_id",
			Message: fmt.Sprintf("%q does not exist", id),
		}
	}
	if err != nil {
		return work.Job{}, unavailable("get job", err)
	}
	return job, nil
}

func (adapter *Adapter) List(ctx context.Context, query work.ListQuery) ([]work.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 1000 {
		return nil, typedInvalid("limit", "must be between 1 and 1000")
	}
	if query.Offset < 0 || query.Offset > 1_000_000 {
		return nil, typedInvalid("offset", "must be between 0 and 1000000")
	}
	conditions := []string{"instance_key = $1"}
	args := []any{adapter.instanceKey}
	if len(query.Statuses) > 0 {
		values := make([]string, len(query.Statuses))
		for index, status := range query.Statuses {
			switch status {
			case work.StatusQueued, work.StatusRunning, work.StatusRetrying, work.StatusPaused,
				work.StatusSucceeded, work.StatusFailed, work.StatusCancelled:
			default:
				return nil, typedInvalid("statuses", fmt.Sprintf("%q is unknown", status))
			}
			values[index] = string(status)
		}
		args = append(args, pq.Array(values))
		conditions = append(conditions, fmt.Sprintf("status = ANY($%d)", len(args)))
	}
	if len(query.Kinds) > 0 {
		values := make([]string, len(query.Kinds))
		for index, kind := range query.Kinds {
			if _, ok := adapter.catalog.Kind(kind); !ok {
				return nil, typedInvalid("kinds", fmt.Sprintf("%q is not registered", kind))
			}
			values[index] = string(kind)
		}
		args = append(args, pq.Array(values))
		conditions = append(conditions, fmt.Sprintf("kind = ANY($%d)", len(args)))
	}
	if len(query.Queues) > 0 {
		registered := make(map[work.Queue]struct{})
		for _, queue := range adapter.catalog.Queues() {
			registered[queue.Key] = struct{}{}
		}
		values := make([]string, len(query.Queues))
		for index, queue := range query.Queues {
			if _, ok := registered[queue]; !ok {
				return nil, typedInvalid("queues", fmt.Sprintf("%q is not registered", queue))
			}
			values[index] = string(queue)
		}
		args = append(args, pq.Array(values))
		conditions = append(conditions, fmt.Sprintf("queue = ANY($%d)", len(args)))
	}
	args = append(args, limit, query.Offset)
	statement := `
SELECT ` + jobColumns + `
FROM work_jobs
WHERE ` + strings.Join(conditions, " AND ") + `
ORDER BY created_at DESC, job_id DESC
LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))
	rows, err := adapter.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, unavailable("list jobs", err)
	}
	defer rows.Close()
	result := make([]work.Job, 0)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, unavailable("scan listed job", err)
		}
		result = append(result, job)
	}
	if err := rows.Err(); err != nil {
		return nil, unavailable("iterate listed jobs", err)
	}
	return result, nil
}

func (adapter *Adapter) Attempts(ctx context.Context, id work.JobID) ([]work.Attempt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var exists bool
	if err := adapter.db.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM work_jobs WHERE instance_key = $1 AND job_id = $2
)`, adapter.instanceKey, id).Scan(&exists); err != nil {
		return nil, unavailable("check job", err)
	}
	if !exists {
		return nil, &work.Error{
			Kind: work.ErrorNotFound, Field: "job_id",
			Message: fmt.Sprintf("%q does not exist", id),
		}
	}
	rows, err := adapter.db.QueryContext(ctx, `
SELECT job_id, attempt_number, worker_id, outcome, error, started_at, finished_at
FROM work_attempts
WHERE instance_key = $1 AND job_id = $2
ORDER BY attempt_number`, adapter.instanceKey, id)
	if err != nil {
		return nil, unavailable("list attempts", err)
	}
	defer rows.Close()
	result := make([]work.Attempt, 0)
	for rows.Next() {
		var item work.Attempt
		var finished sql.NullTime
		if err := rows.Scan(
			&item.JobID, &item.Number, &item.WorkerID, &item.Outcome, &item.Error,
			&item.StartedAt, &finished,
		); err != nil {
			return nil, unavailable("scan attempt", err)
		}
		if finished.Valid {
			item.FinishedAt = timePointer(finished.Time.UTC())
		}
		item.StartedAt = item.StartedAt.UTC()
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, unavailable("iterate attempts", err)
	}
	return result, nil
}

func (adapter *Adapter) Stats(ctx context.Context) (work.Stats, error) {
	if err := ctx.Err(); err != nil {
		return work.Stats{}, err
	}
	now := adapter.clock().UTC()
	rows, err := adapter.db.QueryContext(ctx, `
SELECT status, COUNT(*)
FROM work_jobs
WHERE instance_key = $1
GROUP BY status`, adapter.instanceKey)
	if err != nil {
		return work.Stats{}, unavailable("read status stats", err)
	}
	result := work.Stats{ByStatus: make(map[work.Status]int64)}
	for rows.Next() {
		var status work.Status
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			_ = rows.Close()
			return work.Stats{}, unavailable("scan status stats", err)
		}
		result.ByStatus[status] = count
		if status == work.StatusRunning {
			result.Running = count
		}
	}
	if err := rows.Close(); err != nil {
		return work.Stats{}, unavailable("close status stats", err)
	}
	var oldest sql.NullTime
	if err := adapter.db.QueryRowContext(ctx, `
SELECT COUNT(*), MIN(run_at)
FROM work_jobs
WHERE instance_key = $1
  AND status IN ('queued', 'retrying')
  AND run_at <= $2`, adapter.instanceKey, now).Scan(&result.Due, &oldest); err != nil {
		return work.Stats{}, unavailable("read due stats", err)
	}
	if oldest.Valid {
		result.OldestDueAt = timePointer(oldest.Time)
	}
	return result, nil
}
