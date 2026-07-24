package webhook

import (
	"context"
	"database/sql"
	"time"
)

func (adapter *Postgres) BeginAttempt(ctx context.Context, id DeliveryID) (AttemptPlan, error) {
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return AttemptPlan{}, unavailable("begin attempt", err)
	}
	defer func() { _ = tx.Rollback() }()
	var plan AttemptPlan
	var state DeliveryState
	var endpointState EndpointState
	var attemptCount int
	err = tx.QueryRowContext(ctx, `
SELECT d.delivery_id,d.event_id,e.event_type,d.endpoint_id,r.target_url,ep.secret_ref,e.raw_body,
       e.body_digest,d.attempt_count,d.created_at,d.state,ep.current_state
FROM webhook_deliveries d
JOIN webhook_events e
  ON e.instance_key=d.instance_key AND e.event_id=d.event_id
JOIN webhook_endpoint_revisions r
  ON r.instance_key=d.instance_key AND r.endpoint_id=d.endpoint_id AND r.revision=d.endpoint_revision
JOIN webhook_endpoints ep
  ON ep.instance_key=d.instance_key AND ep.endpoint_id=d.endpoint_id
WHERE d.instance_key=$1 AND d.delivery_id=$2
FOR UPDATE OF d`,
		adapter.instanceKey, id,
	).Scan(&plan.DeliveryID, &plan.EventID, &plan.EventType, &plan.EndpointID,
		&plan.URL, &plan.Secret, &plan.Body, &plan.BodyDigest, &attemptCount,
		&plan.DeliveryCreated, &state, &endpointState)
	if err == sql.ErrNoRows {
		return AttemptPlan{}, &Error{Code: ErrorNotFound, Field: "delivery_id", Message: "does not exist"}
	}
	if err != nil {
		return AttemptPlan{}, unavailable("load attempt plan", err)
	}
	if state == DeliveryDelivered || state == DeliveryFailed || state == DeliveryCancelled {
		return AttemptPlan{}, &Error{Code: ErrorStateConflict, Field: "delivery", Message: "is terminal"}
	}
	now := adapter.clock().UTC()
	if state == DeliveryDelivering {
		var activeAttempt AttemptID
		var startedAt time.Time
		var finished sql.NullTime
		err := tx.QueryRowContext(ctx, `
SELECT attempt_id,started_at,finished_at
FROM webhook_attempts
WHERE instance_key=$1 AND delivery_id=$2
ORDER BY attempt_number DESC LIMIT 1
FOR UPDATE`,
			adapter.instanceKey, id,
		).Scan(&activeAttempt, &startedAt, &finished)
		if err != nil && err != sql.ErrNoRows {
			return AttemptPlan{}, unavailable("load live attempt", err)
		}
		lease := max(2*adapter.catalog.retry.RequestTimeout, time.Minute)
		if err == nil && !finished.Valid && now.Before(startedAt.Add(lease)) {
			return AttemptPlan{}, &Error{Code: ErrorUnavailable, Field: "attempt", Message: "another attempt is still live", Retryable: true}
		}
		if err == nil && !finished.Valid {
			if _, err := tx.ExecContext(ctx, `
UPDATE webhook_attempts
SET outcome='unknown',error_code='lease_expired',finished_at=$4
WHERE instance_key=$1 AND attempt_id=$2 AND delivery_id=$3 AND finished_at IS NULL`,
				adapter.instanceKey, activeAttempt, id, now,
			); err != nil {
				return AttemptPlan{}, unavailable("expire abandoned attempt", err)
			}
		}
	}
	if endpointState == EndpointPaused {
		if _, err := tx.ExecContext(ctx, `
UPDATE webhook_deliveries SET state='paused',updated_at=$3
WHERE instance_key=$1 AND delivery_id=$2`,
			adapter.instanceKey, id, now,
		); err != nil {
			return AttemptPlan{}, unavailable("pause delivery", err)
		}
		if err := tx.Commit(); err != nil {
			return AttemptPlan{}, unavailable("commit paused delivery", err)
		}
		return AttemptPlan{}, &Error{Code: ErrorUnavailable, Field: "endpoint", Message: "is paused", Retryable: true}
	}
	if endpointState == EndpointDisabled || endpointState == EndpointRevoked {
		if _, err := tx.ExecContext(ctx, `
UPDATE webhook_deliveries
SET state='cancelled',last_error_code='endpoint_disabled',updated_at=$3
WHERE instance_key=$1 AND delivery_id=$2`,
			adapter.instanceKey, id, now,
		); err != nil {
			return AttemptPlan{}, unavailable("cancel delivery", err)
		}
		if err := tx.Commit(); err != nil {
			return AttemptPlan{}, unavailable("commit cancelled delivery", err)
		}
		return AttemptPlan{}, &Error{Code: ErrorStateConflict, Field: "endpoint", Message: "is disabled"}
	}
	attemptText, err := NewID()
	if err != nil {
		return AttemptPlan{}, err
	}
	plan.AttemptID = AttemptID(attemptText)
	plan.Number = attemptCount + 1
	if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_attempts(
 instance_key,attempt_id,delivery_id,attempt_number,outcome,request_digest,started_at
) VALUES($1,$2,$3,$4,'unknown',$5,$6)`,
		adapter.instanceKey, plan.AttemptID, id, plan.Number, plan.BodyDigest, now,
	); err != nil {
		return AttemptPlan{}, unavailable("insert attempt", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE webhook_deliveries
SET state='delivering',attempt_count=$3,next_attempt_at=NULL,updated_at=$4
WHERE instance_key=$1 AND delivery_id=$2`,
		adapter.instanceKey, id, plan.Number, now,
	); err != nil {
		return AttemptPlan{}, unavailable("start delivery", err)
	}
	if err := tx.Commit(); err != nil {
		return AttemptPlan{}, unavailable("commit attempt start", err)
	}
	return plan, nil
}

func (adapter *Postgres) CompleteAttempt(ctx context.Context, command AttemptCompletion) (DeliveryView, error) {
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return DeliveryView{}, unavailable("begin attempt completion", err)
	}
	defer func() { _ = tx.Rollback() }()
	var currentState DeliveryState
	if err := tx.QueryRowContext(ctx, `
SELECT state FROM webhook_deliveries
WHERE instance_key=$1 AND delivery_id=$2 FOR UPDATE`,
		adapter.instanceKey, command.Plan.DeliveryID,
	).Scan(&currentState); err != nil {
		if err == sql.ErrNoRows {
			return DeliveryView{}, &Error{Code: ErrorNotFound, Field: "delivery_id", Message: "does not exist"}
		}
		return DeliveryView{}, unavailable("lock delivery completion", err)
	}
	if currentState != DeliveryDelivering {
		return DeliveryView{}, &Error{Code: ErrorStateConflict, Field: "delivery", Message: "is no longer delivering"}
	}
	result, err := tx.ExecContext(ctx, `
UPDATE webhook_attempts
SET outcome=$4,status_code=$5,error_code=$6,response_digest=$7,
    secret_revision=$8,finished_at=$9
WHERE instance_key=$1 AND attempt_id=$2 AND delivery_id=$3 AND finished_at IS NULL`,
		adapter.instanceKey, command.Plan.AttemptID, command.Plan.DeliveryID,
		command.Outcome, command.StatusCode, command.ErrorCode, command.ResponseDigest,
		command.SecretRevision, command.FinishedAt,
	)
	if err != nil {
		return DeliveryView{}, unavailable("complete attempt", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return DeliveryView{}, &Error{Code: ErrorStateConflict, Field: "attempt", Message: "was already completed", Cause: err}
	}
	state := DeliveryFailed
	var next any
	switch command.Outcome {
	case AttemptSucceeded:
		state = DeliveryDelivered
	case AttemptRetryable:
		state, next = DeliveryRetrying, command.NextAttemptAt
	case AttemptPermanent:
		state = DeliveryFailed
	default:
		return DeliveryView{}, invalid(ErrorStateConflict, "outcome", "is invalid")
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE webhook_deliveries
SET state=$3,next_attempt_at=$4,last_error_code=$5,updated_at=$6
WHERE instance_key=$1 AND delivery_id=$2`,
		adapter.instanceKey, command.Plan.DeliveryID, state, next, command.ErrorCode, command.FinishedAt,
	); err != nil {
		return DeliveryView{}, unavailable("update delivery outcome", err)
	}
	if command.DisableEndpoint {
		endpoint, _, err := adapter.loadEndpointTx(ctx, tx, command.Plan.EndpointID)
		if err != nil {
			return DeliveryView{}, err
		}
		if endpoint.State != EndpointRevoked && endpoint.State != EndpointDisabled {
			endpoint.Revision++
			endpoint.State = EndpointDisabled
			endpoint.UpdatedAt = command.FinishedAt
			endpoint.ETag = endpointETag(endpoint)
			if _, err := tx.ExecContext(ctx, `
UPDATE webhook_endpoints
SET current_revision=$3,current_state='disabled',etag=$4,updated_at=$5
WHERE instance_key=$1 AND endpoint_id=$2`,
				adapter.instanceKey, command.Plan.EndpointID, endpoint.Revision, endpoint.ETag, command.FinishedAt,
			); err != nil {
				return DeliveryView{}, unavailable("disable gone endpoint", err)
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_endpoint_revisions(
 instance_key,endpoint_id,revision,target_url,description,state,etag,created_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
				adapter.instanceKey, endpoint.ID, endpoint.Revision, endpoint.URL, endpoint.Description,
				endpoint.State, endpoint.ETag, command.FinishedAt,
			); err != nil {
				return DeliveryView{}, unavailable("record gone endpoint revision", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE webhook_deliveries
SET state='cancelled',last_error_code='endpoint_disabled',updated_at=$3
WHERE instance_key=$1 AND endpoint_id=$2 AND delivery_id<>$4
  AND state IN ('pending','retrying','paused')`,
			adapter.instanceKey, command.Plan.EndpointID, command.FinishedAt, command.Plan.DeliveryID,
		); err != nil {
			return DeliveryView{}, unavailable("cancel disabled endpoint deliveries", err)
		}
	}
	delivery, err := scanDelivery(tx.QueryRowContext(ctx, `
SELECT delivery_id,event_id,endpoint_id,endpoint_revision,subscription_id,subscription_revision,
       state,attempt_count,next_attempt_at,last_error_code,COALESCE(replay_of,''),created_at,updated_at
FROM webhook_deliveries WHERE instance_key=$1 AND delivery_id=$2`,
		adapter.instanceKey, command.Plan.DeliveryID,
	))
	if err != nil {
		return DeliveryView{}, err
	}
	if err := tx.Commit(); err != nil {
		return DeliveryView{}, unavailable("commit attempt completion", err)
	}
	return delivery, nil
}
