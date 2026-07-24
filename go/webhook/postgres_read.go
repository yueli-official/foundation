package webhook

import (
	"context"
	"database/sql"
	"strings"
)

type sqlQueryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (adapter *Postgres) queryer() sqlQueryer {
	if adapter.tx != nil {
		return adapter.tx
	}
	return adapter.db
}

func (adapter *Postgres) Event(ctx context.Context, id EventID) (EventView, error) {
	var value EventView
	err := adapter.queryer().QueryRowContext(ctx, `
SELECT event_id,event_type,subject,raw_body,body_digest,idempotency_key,occurred_at,published_at
FROM webhook_events WHERE instance_key=$1 AND event_id=$2`,
		adapter.instanceKey, id,
	).Scan(&value.ID, &value.Type, &value.Subject, &value.Body, &value.BodyDigest,
		&value.IdempotencyKey, &value.OccurredAt, &value.PublishedAt)
	if err == sql.ErrNoRows {
		return EventView{}, &Error{Code: ErrorNotFound, Field: "event_id", Message: "does not exist"}
	}
	if err != nil {
		return EventView{}, unavailable("load event", err)
	}
	return value, nil
}

func (adapter *Postgres) Delivery(ctx context.Context, id DeliveryID) (DeliveryView, error) {
	return scanDelivery(adapter.queryer().QueryRowContext(ctx, `
SELECT delivery_id,event_id,endpoint_id,endpoint_revision,subscription_id,subscription_revision,
       state,attempt_count,next_attempt_at,last_error_code,COALESCE(replay_of,''),created_at,updated_at
FROM webhook_deliveries WHERE instance_key=$1 AND delivery_id=$2`,
		adapter.instanceKey, id,
	))
}

type rowScanner interface{ Scan(...any) error }

func scanDelivery(row rowScanner) (DeliveryView, error) {
	var value DeliveryView
	var next sql.NullTime
	err := row.Scan(&value.ID, &value.EventID, &value.EndpointID, &value.EndpointRevision,
		&value.SubscriptionID, &value.SubscriptionRevision, &value.State, &value.AttemptCount,
		&next, &value.LastErrorCode, &value.ReplayOf, &value.CreatedAt, &value.UpdatedAt)
	if err == sql.ErrNoRows {
		return DeliveryView{}, &Error{Code: ErrorNotFound, Field: "delivery_id", Message: "does not exist"}
	}
	if err != nil {
		return DeliveryView{}, unavailable("load delivery", err)
	}
	if next.Valid {
		value.NextAttemptAt = next.Time
	}
	return value, nil
}

func (adapter *Postgres) ListDeliveries(ctx context.Context, query DeliveryQuery) (DeliveryPage, error) {
	limit := query.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 500 {
		return DeliveryPage{}, invalid(ErrorLimitExceeded, "limit", "must be between 1 and 500")
	}
	var states []string
	for _, state := range query.States {
		states = append(states, string(state))
	}
	rows, err := adapter.queryer().QueryContext(ctx, `
SELECT d.delivery_id,d.event_id,d.endpoint_id,d.endpoint_revision,d.subscription_id,d.subscription_revision,
       d.state,d.attempt_count,d.next_attempt_at,d.last_error_code,COALESCE(d.replay_of,''),d.created_at,d.updated_at
FROM webhook_deliveries d
JOIN webhook_events e ON e.instance_key=d.instance_key AND e.event_id=d.event_id
WHERE d.instance_key=$1
  AND ($2='' OR d.endpoint_id=$2)
  AND ($3='' OR e.event_type=$3)
  AND ($4::text[] IS NULL OR d.state=ANY($4))
  AND ($5::timestamptz IS NULL OR d.created_at >= $5)
  AND ($6::timestamptz IS NULL OR d.created_at <= $6)
  AND ($7='' OR d.delivery_id > $7)
ORDER BY d.delivery_id
LIMIT $8`,
		adapter.instanceKey, query.EndpointID, query.EventType, nullableStrings(states),
		nullableTime(query.Since), nullableTime(query.Until), query.After, limit+1,
	)
	if err != nil {
		return DeliveryPage{}, unavailable("list deliveries", err)
	}
	defer rows.Close()
	var values []DeliveryView
	for rows.Next() {
		value, err := scanDelivery(rows)
		if err != nil {
			return DeliveryPage{}, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return DeliveryPage{}, unavailable("iterate deliveries", err)
	}
	page := DeliveryPage{}
	if len(values) > limit {
		page.Next, values = string(values[limit-1].ID), values[:limit]
	}
	page.Deliveries = values
	return page, nil
}

func (adapter *Postgres) Attempts(ctx context.Context, id DeliveryID) ([]AttemptView, error) {
	rows, err := adapter.queryer().QueryContext(ctx, `
SELECT attempt_id,delivery_id,attempt_number,outcome,status_code,error_code,request_digest,
       response_digest,secret_revision,started_at,finished_at
FROM webhook_attempts WHERE instance_key=$1 AND delivery_id=$2 ORDER BY attempt_number`,
		adapter.instanceKey, id,
	)
	if err != nil {
		return nil, unavailable("list attempts", err)
	}
	defer rows.Close()
	var values []AttemptView
	for rows.Next() {
		var value AttemptView
		var finished sql.NullTime
		if err := rows.Scan(&value.ID, &value.DeliveryID, &value.Number, &value.Outcome,
			&value.StatusCode, &value.ErrorCode, &value.RequestDigest, &value.ResponseDigest,
			&value.SecretRevision, &value.StartedAt, &finished); err != nil {
			return nil, unavailable("scan attempt", err)
		}
		if finished.Valid {
			value.FinishedAt = &finished.Time
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, unavailable("iterate attempts", err)
	}
	if len(values) == 0 {
		var exists bool
		if err := adapter.queryer().QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM webhook_deliveries WHERE instance_key=$1 AND delivery_id=$2)`,
			adapter.instanceKey, id,
		).Scan(&exists); err != nil {
			return nil, unavailable("check delivery", err)
		}
		if !exists {
			return nil, &Error{Code: ErrorNotFound, Field: "delivery_id", Message: "does not exist"}
		}
	}
	return values, nil
}

func (adapter *Postgres) Snapshot(ctx context.Context) (MetricsSnapshot, error) {
	result := MetricsSnapshot{ByState: map[DeliveryState]int64{}}
	rows, err := adapter.queryer().QueryContext(ctx, `
SELECT state,count(*) FROM webhook_deliveries WHERE instance_key=$1 GROUP BY state`,
		adapter.instanceKey,
	)
	if err != nil {
		return MetricsSnapshot{}, unavailable("snapshot delivery states", err)
	}
	for rows.Next() {
		var state DeliveryState
		var count int64
		if err := rows.Scan(&state, &count); err != nil {
			rows.Close()
			return MetricsSnapshot{}, unavailable("scan state", err)
		}
		result.ByState[state] = count
	}
	if err := rows.Close(); err != nil {
		return MetricsSnapshot{}, unavailable("close snapshot", err)
	}
	var oldest sql.NullTime
	if err := adapter.queryer().QueryRowContext(ctx, `
SELECT count(*),min(next_attempt_at)
FROM webhook_deliveries
WHERE instance_key=$1 AND state IN ('pending','retrying') AND next_attempt_at <= now()`,
		adapter.instanceKey,
	).Scan(&result.Due, &oldest); err != nil {
		return MetricsSnapshot{}, unavailable("snapshot due", err)
	}
	if oldest.Valid {
		result.OldestDueAt = &oldest.Time
	}
	return result, nil
}

func nullableStrings(values []string) any {
	if len(values) == 0 {
		return nil
	}
	return "{" + strings.Join(values, ",") + "}"
}

func nullableTime(value interface{ IsZero() bool }) any {
	if value.IsZero() {
		return nil
	}
	return value
}
