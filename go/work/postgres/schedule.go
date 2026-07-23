package postgres

import (
	"context"
	"time"

	"github.com/yueli-official/foundation/go/work"
)

func (adapter *Adapter) MaterializeSchedules(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	now := adapter.clock().UTC()
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, unavailable("begin schedule materialization", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
SELECT schedule_key, next_run_at
FROM work_schedules
WHERE instance_key = $1 AND next_run_at <= $2
ORDER BY next_run_at, schedule_key
FOR UPDATE SKIP LOCKED`, adapter.instanceKey, now)
	if err != nil {
		return 0, unavailable("find due schedules", err)
	}
	type dueSchedule struct {
		key  string
		next time.Time
	}
	due := make([]dueSchedule, 0)
	for rows.Next() {
		var item dueSchedule
		if err := rows.Scan(&item.key, &item.next); err != nil {
			_ = rows.Close()
			return 0, unavailable("scan due schedule", err)
		}
		due = append(due, item)
	}
	if err := rows.Close(); err != nil {
		return 0, unavailable("close due schedules", err)
	}
	created := 0
	for _, item := range due {
		next := item.next.UTC()
		for !next.After(now) && created < adapter.catalog.Limits().MaxScheduleCatchUp {
			request, ok := adapter.catalog.ScheduleRequest(item.key, next)
			if !ok {
				return 0, &work.Error{
					Kind: work.ErrorConflict, Field: "schedule",
					Message: "database contains a schedule absent from the compiled catalog",
				}
			}
			prepared, err := adapter.catalog.Prepare(now, request)
			if err != nil {
				return 0, err
			}
			result, err := adapter.enqueuePreparedTx(ctx, tx, now, prepared, "")
			if err != nil {
				return 0, err
			}
			if !result.Replay {
				created++
			}
			following, ok := adapter.catalog.NextSchedule(item.key, next)
			if !ok {
				return 0, &work.Error{
					Kind: work.ErrorConflict, Field: "schedule",
					Message: "compiled schedule disappeared",
				}
			}
			next = following
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE work_schedules
SET next_run_at = $3, updated_at = $4
WHERE instance_key = $1 AND schedule_key = $2`,
			adapter.instanceKey, item.key, next, now); err != nil {
			return 0, unavailable("advance schedule cursor", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, unavailable("commit schedule materialization", err)
	}
	return created, nil
}
