package webhook

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (adapter *Postgres) SetEndpointState(ctx context.Context, command SetEndpointStateCommand) (Endpoint, error) {
	if command.State != EndpointActive && command.State != EndpointPaused &&
		command.State != EndpointDisabled && command.State != EndpointRevoked {
		return Endpoint{}, invalid(ErrorStateConflict, "state", "is invalid")
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return Endpoint{}, unavailable("begin endpoint state", err)
	}
	defer func() { _ = tx.Rollback() }()
	endpoint, _, err := adapter.loadEndpointTx(ctx, tx, command.EndpointID)
	if err != nil {
		return Endpoint{}, err
	}
	if command.ExpectedETag != endpoint.ETag {
		return Endpoint{}, &Error{Code: ErrorETagConflict, Field: "expected_etag", Message: "does not match"}
	}
	if endpoint.State == EndpointRevoked {
		return Endpoint{}, &Error{Code: ErrorStateConflict, Field: "state", Message: "revoked endpoint cannot transition"}
	}
	now := adapter.clock().UTC()
	endpoint.Revision++
	endpoint.State = command.State
	endpoint.UpdatedAt = now
	endpoint.ETag = endpointETag(endpoint)
	if _, err := tx.ExecContext(ctx, `
UPDATE webhook_endpoints
SET current_revision=$3,current_state=$4,etag=$5,updated_at=$6
WHERE instance_key=$1 AND endpoint_id=$2`,
		adapter.instanceKey, endpoint.ID, endpoint.Revision, endpoint.State, endpoint.ETag, now,
	); err != nil {
		return Endpoint{}, unavailable("update endpoint state", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_endpoint_revisions(
 instance_key,endpoint_id,revision,target_url,description,state,etag,created_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		adapter.instanceKey, endpoint.ID, endpoint.Revision, endpoint.URL, endpoint.Description,
		endpoint.State, endpoint.ETag, now,
	); err != nil {
		return Endpoint{}, unavailable("insert endpoint state revision", err)
	}
	if command.State == EndpointPaused {
		if _, err := tx.ExecContext(ctx, `
UPDATE webhook_deliveries SET state='paused',updated_at=$3
WHERE instance_key=$1 AND endpoint_id=$2 AND state IN ('pending','retrying')`,
			adapter.instanceKey, endpoint.ID, now,
		); err != nil {
			return Endpoint{}, unavailable("pause deliveries", err)
		}
	}
	if command.State == EndpointDisabled || command.State == EndpointRevoked {
		if _, err := tx.ExecContext(ctx, `
UPDATE webhook_deliveries SET state='cancelled',last_error_code='endpoint_disabled',updated_at=$3
WHERE instance_key=$1 AND endpoint_id=$2 AND state IN ('pending','retrying','paused')`,
			adapter.instanceKey, endpoint.ID, now,
		); err != nil {
			return Endpoint{}, unavailable("cancel deliveries", err)
		}
	}
	if command.State == EndpointActive {
		rows, err := tx.QueryContext(ctx, `
UPDATE webhook_deliveries SET state='pending',next_attempt_at=$3,updated_at=$3
WHERE instance_key=$1 AND endpoint_id=$2 AND state='paused'
RETURNING delivery_id`,
			adapter.instanceKey, endpoint.ID, now)
		if err != nil {
			return Endpoint{}, unavailable("resume deliveries", err)
		}
		var resumed []DeliveryID
		for rows.Next() {
			var deliveryID DeliveryID
			if err := rows.Scan(&deliveryID); err != nil {
				_ = rows.Close()
				return Endpoint{}, unavailable("scan resumed delivery", err)
			}
			resumed = append(resumed, deliveryID)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return Endpoint{}, unavailable("iterate resumed deliveries", err)
		}
		if err := rows.Close(); err != nil {
			return Endpoint{}, unavailable("close resumed deliveries", err)
		}
		for _, deliveryID := range resumed {
			if err := adapter.scheduler.EnqueueTx(ctx, tx, DeliveryWork{
				DeliveryID: deliveryID,
				RunAt:      now,
				Key:        fmt.Sprintf("webhook.delivery:%s:resume:%d", deliveryID, endpoint.Revision),
			}); err != nil {
				return Endpoint{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return Endpoint{}, unavailable("commit endpoint state", err)
	}
	return endpoint, nil
}

func (adapter *Postgres) RotateSecret(ctx context.Context, command RotateSecretCommand) (EndpointCredential, error) {
	if command.Overlap <= 0 || command.Overlap > 7*24*time.Hour {
		return EndpointCredential{}, invalid(ErrorInvalidEvent, "overlap", "must be positive and at most seven days")
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return EndpointCredential{}, unavailable("begin secret rotation", err)
	}
	defer func() { _ = tx.Rollback() }()
	endpoint, secretRef, err := adapter.loadEndpointTx(ctx, tx, command.EndpointID)
	if err != nil {
		return EndpointCredential{}, err
	}
	if endpoint.State == EndpointRevoked {
		return EndpointCredential{}, &Error{Code: ErrorStateConflict, Field: "state", Message: "endpoint is revoked"}
	}
	plaintext, err := newSecret()
	if err != nil {
		return EndpointCredential{}, err
	}
	decoded, _ := decodeSecret(plaintext)
	now := adapter.clock().UTC()
	nextRevision := endpoint.Revision + 1
	if err := adapter.secrets.Rotate(ctx, secretRef, SecretMaterial{
		Revision: SecretRevision(fmt.Sprintf("r%d", nextRevision)), Value: decoded, NotBefore: now,
	}, now.Add(command.Overlap)); err != nil {
		return EndpointCredential{}, err
	}
	endpoint.Revision = nextRevision
	endpoint.UpdatedAt = now
	endpoint.ETag = endpointETag(endpoint)
	if _, err := tx.ExecContext(ctx, `
UPDATE webhook_endpoints SET current_revision=$3,etag=$4,updated_at=$5
WHERE instance_key=$1 AND endpoint_id=$2`,
		adapter.instanceKey, endpoint.ID, endpoint.Revision, endpoint.ETag, now,
	); err != nil {
		return EndpointCredential{}, unavailable("update rotated endpoint", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_endpoint_revisions(
 instance_key,endpoint_id,revision,target_url,description,state,etag,created_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		adapter.instanceKey, endpoint.ID, endpoint.Revision, endpoint.URL, endpoint.Description,
		endpoint.State, endpoint.ETag, now,
	); err != nil {
		return EndpointCredential{}, unavailable("insert rotation revision", err)
	}
	if err := tx.Commit(); err != nil {
		return EndpointCredential{}, unavailable("commit secret rotation", err)
	}
	return EndpointCredential{Endpoint: endpoint, Secret: plaintext}, nil
}

func (adapter *Postgres) loadEndpointTx(
	ctx context.Context,
	tx *sql.Tx,
	id EndpointID,
) (Endpoint, SecretRef, error) {
	var endpoint Endpoint
	var secretRef SecretRef
	err := tx.QueryRowContext(ctx, `
SELECT e.endpoint_id,e.current_revision,r.target_url,r.description,e.current_state,e.etag,
       e.secret_ref,e.created_at,e.updated_at
FROM webhook_endpoints e
JOIN webhook_endpoint_revisions r
  ON r.instance_key=e.instance_key AND r.endpoint_id=e.endpoint_id AND r.revision=e.current_revision
WHERE e.instance_key=$1 AND e.endpoint_id=$2 FOR UPDATE`,
		adapter.instanceKey, id,
	).Scan(&endpoint.ID, &endpoint.Revision, &endpoint.URL, &endpoint.Description,
		&endpoint.State, &endpoint.ETag, &secretRef, &endpoint.CreatedAt, &endpoint.UpdatedAt)
	if err == sql.ErrNoRows {
		return Endpoint{}, "", &Error{Code: ErrorNotFound, Field: "endpoint_id", Message: "does not exist"}
	}
	if err != nil {
		return Endpoint{}, "", unavailable("load endpoint", err)
	}
	return endpoint, secretRef, nil
}

func replayKey(command ReplayCommand) string {
	return strings.TrimSpace(command.IdempotencyKey)
}
