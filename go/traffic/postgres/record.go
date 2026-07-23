package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/yueli-official/foundation/go/traffic"
)

type receiptRecord struct {
	eventID              traffic.EventID
	fingerprint          []byte
	resource             traffic.Resource
	counted              bool
	dropReason           traffic.DropReason
	firstInstanceVisitor bool
	firstResourceVisitor bool
}

func (adapter *Adapter) Record(ctx context.Context, observation traffic.Observation) (traffic.RecordResult, error) {
	results, err := adapter.RecordBatch(ctx, []traffic.Observation{observation})
	if err != nil {
		return traffic.RecordResult{}, err
	}
	return results[0], nil
}

func (adapter *Adapter) RecordBatch(ctx context.Context, observations []traffic.Observation) ([]traffic.RecordResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(observations) == 0 {
		return nil, typedInvalid("observations", "must contain at least one item")
	}
	if len(observations) > adapter.catalog.Limits().MaxBatchSize {
		return nil, typedInvalid("observations", fmt.Sprintf("exceeds max batch size %d", adapter.catalog.Limits().MaxBatchSize))
	}
	now := adapter.clock().UTC()
	prepared := make([]traffic.PreparedObservation, len(observations))
	for index, observation := range observations {
		value, err := adapter.catalog.PrepareObservation(now, observation)
		if err != nil {
			return nil, indexedError(err, "observations", index)
		}
		prepared[index] = value
	}

	seen := make(map[traffic.EventID][32]byte, len(prepared))
	for _, value := range prepared {
		if fingerprint, ok := seen[value.EventID]; ok && fingerprint != value.Fingerprint {
			return nil, typedConflict("event_id", fmt.Sprintf("%q is reused with a different payload", value.EventID))
		}
		seen[value.EventID] = value.Fingerprint
	}

	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, unavailable("begin record batch", err)
	}
	defer func() { _ = tx.Rollback() }()
	results := make([]traffic.RecordResult, 0, len(prepared))
	for _, value := range prepared {
		inserted, err := adapter.insertReceipt(ctx, tx, value, now)
		if err != nil {
			return nil, unavailable("insert event receipt", err)
		}
		if !inserted {
			receipt, err := adapter.loadReceipt(ctx, tx, value.EventID)
			if err != nil {
				return nil, unavailable("load event receipt", err)
			}
			if !sameFingerprint(receipt.fingerprint, value.Fingerprint) {
				return nil, typedConflict("event_id", fmt.Sprintf("%q is reused with a different payload", value.EventID))
			}
			result, err := adapter.resultForReceipt(ctx, tx, receipt, true)
			if err != nil {
				return nil, unavailable("read replay totals", err)
			}
			results = append(results, result)
			continue
		}

		receipt := receiptRecord{
			eventID: value.EventID, fingerprint: value.Fingerprint[:],
			resource: value.Resource, counted: value.Counted, dropReason: value.DropReason,
		}
		if value.Counted {
			firstInstance, firstResource, err := adapter.applyCounted(ctx, tx, value, now)
			if err != nil {
				return nil, unavailable("aggregate event", err)
			}
			receipt.firstInstanceVisitor = firstInstance
			receipt.firstResourceVisitor = firstResource
			if _, err := tx.ExecContext(ctx, `
UPDATE traffic_event_receipts
SET first_instance_visitor = $3, first_resource_visitor = $4
WHERE instance_key = $1 AND event_id = $2`,
				adapter.instanceKey, value.EventID, firstInstance, firstResource,
			); err != nil {
				return nil, unavailable("finalize event receipt", err)
			}
		}
		result, err := adapter.resultForReceipt(ctx, tx, receipt, false)
		if err != nil {
			return nil, unavailable("read record totals", err)
		}
		results = append(results, result)
	}
	if err := tx.Commit(); err != nil {
		return nil, unavailable("commit record batch", err)
	}
	return results, nil
}

func indexedError(err error, field string, index int) error {
	var typed *traffic.Error
	if !errors.As(err, &typed) {
		return err
	}
	copy := *typed
	if copy.Field == "" {
		copy.Field = fmt.Sprintf("%s[%d]", field, index)
	} else {
		copy.Field = fmt.Sprintf("%s[%d].%s", field, index, copy.Field)
	}
	return &copy
}

func (adapter *Adapter) insertReceipt(
	ctx context.Context,
	tx *sql.Tx,
	value traffic.PreparedObservation,
	receivedAt time.Time,
) (bool, error) {
	var visitor any
	if value.HasVisitor {
		visitor = value.VisitorToken[:]
	}
	var inserted int
	err := tx.QueryRowContext(ctx, `
INSERT INTO traffic_event_receipts (
    instance_key, event_id, fingerprint, resource_kind, resource_id,
    occurred_at, metric_day, visit_class, visitor_token, counted, drop_reason, received_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7::date, $8, $9, $10, $11, $12)
ON CONFLICT (instance_key, event_id) DO NOTHING
RETURNING 1`,
		adapter.instanceKey, value.EventID, value.Fingerprint[:],
		value.Resource.Kind, value.Resource.ID, value.OccurredAt, value.Day.String(),
		value.Class, visitor, value.Counted, value.DropReason, receivedAt,
	).Scan(&inserted)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return inserted == 1, nil
}

func (adapter *Adapter) loadReceipt(ctx context.Context, tx *sql.Tx, eventID traffic.EventID) (receiptRecord, error) {
	var value receiptRecord
	value.eventID = eventID
	err := tx.QueryRowContext(ctx, `
SELECT fingerprint, resource_kind, resource_id, counted, drop_reason,
       first_instance_visitor, first_resource_visitor
FROM traffic_event_receipts
WHERE instance_key = $1 AND event_id = $2`,
		adapter.instanceKey, eventID,
	).Scan(
		&value.fingerprint, &value.resource.Kind, &value.resource.ID,
		&value.counted, &value.dropReason,
		&value.firstInstanceVisitor, &value.firstResourceVisitor,
	)
	return value, err
}

func (adapter *Adapter) applyCounted(
	ctx context.Context,
	tx *sql.Tx,
	value traffic.PreparedObservation,
	now time.Time,
) (bool, bool, error) {
	scopes := []scopeColumns{instanceColumns(), columnsForResource(value.Resource)}
	for _, scope := range scopes {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO traffic_totals (
    instance_key, scope_kind, resource_kind, resource_id, views, unique_visitor_days, updated_at
)
VALUES ($1, $2, $3, $4, 1, 0, $5)
ON CONFLICT (instance_key, scope_kind, resource_kind, resource_id)
DO UPDATE SET
    views = traffic_totals.views + 1,
    updated_at = EXCLUDED.updated_at`,
			adapter.instanceKey, scope.kind, scope.resourceKind, scope.resourceID, now,
		); err != nil {
			return false, false, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO traffic_daily (
    instance_key, metric_day, scope_kind, resource_kind, resource_id,
    views, unique_visitor_days, updated_at
)
VALUES ($1, $2::date, $3, $4, $5, 1, 0, $6)
ON CONFLICT (instance_key, metric_day, scope_kind, resource_kind, resource_id)
DO UPDATE SET
    views = traffic_daily.views + 1,
    updated_at = EXCLUDED.updated_at`,
			adapter.instanceKey, value.Day.String(), scope.kind,
			scope.resourceKind, scope.resourceID, now,
		); err != nil {
			return false, false, err
		}
	}
	if !value.HasVisitor {
		return false, false, nil
	}
	first := [2]bool{}
	for index, scope := range scopes {
		inserted, err := insertVisitorMarker(ctx, tx, adapter.instanceKey, value, scope)
		if err != nil {
			return false, false, err
		}
		first[index] = inserted
		if !inserted {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE traffic_totals
SET unique_visitor_days = unique_visitor_days + 1, updated_at = $5
WHERE instance_key = $1 AND scope_kind = $2 AND resource_kind = $3 AND resource_id = $4`,
			adapter.instanceKey, scope.kind, scope.resourceKind, scope.resourceID, now,
		); err != nil {
			return false, false, err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE traffic_daily
SET unique_visitor_days = unique_visitor_days + 1, updated_at = $6
WHERE instance_key = $1 AND metric_day = $2::date
  AND scope_kind = $3 AND resource_kind = $4 AND resource_id = $5`,
			adapter.instanceKey, value.Day.String(), scope.kind,
			scope.resourceKind, scope.resourceID, now,
		); err != nil {
			return false, false, err
		}
	}
	return first[0], first[1], nil
}

func insertVisitorMarker(
	ctx context.Context,
	tx *sql.Tx,
	instanceKey string,
	value traffic.PreparedObservation,
	scope scopeColumns,
) (bool, error) {
	var inserted int
	err := tx.QueryRowContext(ctx, `
INSERT INTO traffic_visitor_markers (
    instance_key, metric_day, scope_kind, resource_kind, resource_id,
    visitor_token, first_seen_at
)
VALUES ($1, $2::date, $3, $4, $5, $6, $7)
ON CONFLICT (
    instance_key, metric_day, scope_kind, resource_kind, resource_id, visitor_token
) DO NOTHING
RETURNING 1`,
		instanceKey, value.Day.String(), scope.kind, scope.resourceKind,
		scope.resourceID, value.VisitorToken[:], value.OccurredAt,
	).Scan(&inserted)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return inserted == 1, nil
}

func (adapter *Adapter) resultForReceipt(
	ctx context.Context,
	tx *sql.Tx,
	receipt receiptRecord,
	replay bool,
) (traffic.RecordResult, error) {
	instanceTotals, err := readTotals(ctx, tx, adapter.instanceKey, instanceColumns())
	if err != nil {
		return traffic.RecordResult{}, err
	}
	resourceTotals, err := readTotals(ctx, tx, adapter.instanceKey, columnsForResource(receipt.resource))
	if err != nil {
		return traffic.RecordResult{}, err
	}
	return traffic.RecordResult{
		EventID: receipt.eventID, Counted: receipt.counted, Replay: replay,
		DropReason:           receipt.dropReason,
		FirstInstanceVisitor: receipt.firstInstanceVisitor,
		FirstResourceVisitor: receipt.firstResourceVisitor,
		InstanceTotals:       instanceTotals, ResourceTotals: resourceTotals,
	}, nil
}
