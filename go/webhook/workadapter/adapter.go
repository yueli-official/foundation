package workadapter

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/yueli-official/foundation/go/webhook"
	"github.com/yueli-official/foundation/go/work"
	workpostgres "github.com/yueli-official/foundation/go/work/postgres"
)

type Adapter struct {
	Work *workpostgres.Adapter
}

var _ webhook.TransactionalScheduler = (*Adapter)(nil)

func (adapter *Adapter) Enqueue(ctx context.Context, request webhook.DeliveryWork) error {
	if adapter == nil || adapter.Work == nil {
		return &webhook.Error{Code: webhook.ErrorUnavailable, Field: "work", Message: "adapter is not configured", Retryable: true}
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	_, err = adapter.Work.Enqueue(ctx, work.Request{
		Kind: webhook.WorkKind, Payload: payload, RunAt: request.RunAt,
		IdempotencyKey: request.Key,
	})
	return err
}

func (adapter *Adapter) EnqueueTx(ctx context.Context, tx *sql.Tx, request webhook.DeliveryWork) error {
	if adapter == nil || adapter.Work == nil {
		return &webhook.Error{Code: webhook.ErrorUnavailable, Field: "work", Message: "adapter is not configured", Retryable: true}
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	_, err = adapter.Work.EnqueueTx(ctx, tx, work.Request{
		Kind: webhook.WorkKind, Payload: payload, RunAt: request.RunAt,
		IdempotencyKey: request.Key,
	})
	return err
}
