package webhook

import (
	"context"
	"database/sql"
	"strings"
)

func (adapter *Postgres) Verify(ctx context.Context, source InboundSource, message IncomingMessage) (VerifiedInbound, error) {
	return verifyInbound(ctx, adapter.catalog, adapter.secrets, adapter.clock, source, message)
}

func (adapter *Postgres) Accept(ctx context.Context, verified VerifiedInbound) (InboundReceipt, error) {
	if adapter.tx != nil {
		return adapter.acceptTx(ctx, adapter.tx, verified)
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return InboundReceipt{}, unavailable("begin inbound accept", err)
	}
	defer func() { _ = tx.Rollback() }()
	receipt, err := adapter.acceptTx(ctx, tx, verified)
	if err != nil {
		return InboundReceipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return InboundReceipt{}, unavailable("commit inbound accept", err)
	}
	return receipt, nil
}

func (adapter *Postgres) AcceptTx(ctx context.Context, tx *sql.Tx, verified VerifiedInbound) (InboundReceipt, error) {
	if tx == nil {
		return InboundReceipt{}, invalid(ErrorInvalidEvent, "tx", "is required")
	}
	return adapter.acceptTx(ctx, tx, verified)
}

func (adapter *Postgres) acceptTx(ctx context.Context, tx *sql.Tx, verified VerifiedInbound) (InboundReceipt, error) {
	if verified.catalogDigest != adapter.catalog.digest || verified.source == "" || verified.eventID == "" {
		return InboundReceipt{}, invalid(ErrorSignatureInvalid, "verified", "was not produced by this runtime")
	}
	var receipt InboundReceipt
	err := tx.QueryRowContext(ctx, `
SELECT receipt_id,inbound_source,event_id,body_digest,secret_revision,received_at,accepted_at
FROM webhook_inbound_receipts
WHERE instance_key=$1 AND inbound_source=$2 AND event_id=$3`,
		adapter.instanceKey, verified.source, verified.eventID,
	).Scan(&receipt.ReceiptID, &receipt.Source, &receipt.EventID, &receipt.BodyDigest,
		&receipt.KeyRevision, &receipt.ReceivedAt, &receipt.AcceptedAt)
	switch {
	case err == nil:
		if receipt.BodyDigest != verified.bodyDigest {
			return InboundReceipt{}, &Error{Code: ErrorIdempotency, Field: "event_id", Message: "was previously accepted with different content"}
		}
		receipt.FirstSeen = false
		return receipt, nil
	case err != sql.ErrNoRows:
		return InboundReceipt{}, unavailable("lookup inbound receipt", err)
	}
	id, err := NewID()
	if err != nil {
		return InboundReceipt{}, err
	}
	receipt = InboundReceipt{
		ReceiptID: id, Source: verified.source, EventID: verified.eventID,
		BodyDigest: verified.bodyDigest, KeyRevision: verified.secretRevision,
		FirstSeen: true, ReceivedAt: verified.receivedAt, AcceptedAt: adapter.clock().UTC(),
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_inbound_receipts(
 instance_key,receipt_id,inbound_source,event_id,body_digest,secret_revision,received_at,accepted_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		adapter.instanceKey, receipt.ReceiptID, receipt.Source, receipt.EventID,
		receipt.BodyDigest, receipt.KeyRevision, receipt.ReceivedAt, receipt.AcceptedAt,
	); err != nil {
		return InboundReceipt{}, unavailable("insert inbound receipt", err)
	}
	return receipt, nil
}

func (adapter *Postgres) Replay(ctx context.Context, command ReplayCommand) (ReplayReceipt, error) {
	key := replayKey(command)
	if key == "" || strings.TrimSpace(command.Reason) == "" {
		return ReplayReceipt{}, invalid(ErrorInvalidEvent, "replay", "reason and idempotency key are required")
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return ReplayReceipt{}, unavailable("begin replay", err)
	}
	defer func() { _ = tx.Rollback() }()
	var existingID DeliveryID
	err = tx.QueryRowContext(ctx, `
SELECT replay_delivery_id FROM webhook_replay_receipts WHERE instance_key=$1 AND idempotency_key=$2`,
		adapter.instanceKey, key,
	).Scan(&existingID)
	if err == nil {
		delivery, loadErr := scanDelivery(tx.QueryRowContext(ctx, `
SELECT delivery_id,event_id,endpoint_id,endpoint_revision,subscription_id,subscription_revision,
       state,attempt_count,next_attempt_at,last_error_code,COALESCE(replay_of,''),created_at,updated_at
FROM webhook_deliveries WHERE instance_key=$1 AND delivery_id=$2`,
			adapter.instanceKey, existingID,
		))
		if loadErr != nil {
			return ReplayReceipt{}, loadErr
		}
		return ReplayReceipt{Delivery: delivery, Duplicate: true}, nil
	}
	if err != sql.ErrNoRows {
		return ReplayReceipt{}, unavailable("lookup replay", err)
	}
	original, err := scanDelivery(tx.QueryRowContext(ctx, `
SELECT delivery_id,event_id,endpoint_id,endpoint_revision,subscription_id,subscription_revision,
       state,attempt_count,next_attempt_at,last_error_code,COALESCE(replay_of,''),created_at,updated_at
FROM webhook_deliveries WHERE instance_key=$1 AND delivery_id=$2 FOR UPDATE`,
		adapter.instanceKey, command.DeliveryID,
	))
	if err != nil {
		return ReplayReceipt{}, err
	}
	if original.State != DeliveryFailed && original.State != DeliveryCancelled {
		return ReplayReceipt{}, &Error{Code: ErrorStateConflict, Field: "delivery", Message: "is not replayable"}
	}
	var endpointRevision uint64
	var endpointState EndpointState
	if err := tx.QueryRowContext(ctx, `
SELECT current_revision,current_state FROM webhook_endpoints
WHERE instance_key=$1 AND endpoint_id=$2`,
		adapter.instanceKey, original.EndpointID,
	).Scan(&endpointRevision, &endpointState); err != nil {
		return ReplayReceipt{}, unavailable("load replay endpoint", err)
	}
	if endpointState != EndpointActive {
		return ReplayReceipt{}, &Error{Code: ErrorStateConflict, Field: "endpoint", Message: "is not active"}
	}
	idText, err := NewID()
	if err != nil {
		return ReplayReceipt{}, err
	}
	id := DeliveryID(idText)
	now := adapter.clock().UTC()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_deliveries(
 instance_key,delivery_id,event_id,endpoint_id,endpoint_revision,subscription_id,
 subscription_revision,state,attempt_count,next_attempt_at,replay_of,created_at,updated_at
) VALUES($1,$2,$3,$4,$5,$6,$7,'pending',0,$8,$9,$8,$8)`,
		adapter.instanceKey, id, original.EventID, original.EndpointID, endpointRevision,
		original.SubscriptionID, original.SubscriptionRevision, now, original.ID,
	); err != nil {
		return ReplayReceipt{}, unavailable("insert replay delivery", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_replay_receipts(
 instance_key,idempotency_key,original_delivery_id,replay_delivery_id,reason,created_at
) VALUES($1,$2,$3,$4,$5,$6)`,
		adapter.instanceKey, key, original.ID, id, strings.TrimSpace(command.Reason), now,
	); err != nil {
		return ReplayReceipt{}, unavailable("insert replay receipt", err)
	}
	if err := adapter.scheduler.EnqueueTx(ctx, tx, DeliveryWork{
		DeliveryID: id, RunAt: now, Key: "webhook.delivery:" + string(id),
	}); err != nil {
		return ReplayReceipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReplayReceipt{}, unavailable("commit replay", err)
	}
	delivery := original
	delivery.ID, delivery.EndpointRevision, delivery.State = id, endpointRevision, DeliveryPending
	delivery.AttemptCount, delivery.NextAttemptAt, delivery.LastErrorCode = 0, now, ""
	delivery.ReplayOf, delivery.CreatedAt, delivery.UpdatedAt = original.ID, now, now
	return ReplayReceipt{Delivery: delivery}, nil
}
