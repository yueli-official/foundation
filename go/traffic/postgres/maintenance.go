package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/yueli-official/foundation/go/traffic"
)

func (adapter *Adapter) ImportBaseline(ctx context.Context, command traffic.BaselineImport) (traffic.ImportResult, error) {
	if err := ctx.Err(); err != nil {
		return traffic.ImportResult{}, err
	}
	prepared, err := prepareBaseline(adapter.catalog, command)
	if err != nil {
		return traffic.ImportResult{}, err
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return traffic.ImportResult{}, unavailable("begin baseline import", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := importBaselineTx(ctx, tx, adapter.instanceKey, adapter.clock().UTC(), prepared)
	if err != nil {
		return traffic.ImportResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return traffic.ImportResult{}, unavailable("commit baseline import", err)
	}
	return result, nil
}

type preparedBaseline struct {
	source      string
	resource    traffic.Resource
	views       int64
	fingerprint [32]byte
}

func prepareBaseline(catalog *traffic.Catalog, command traffic.BaselineImport) (preparedBaseline, error) {
	source := strings.TrimSpace(command.Source)
	if source == "" {
		return preparedBaseline{}, typedInvalid("source", "is required")
	}
	if len(source) > catalog.Limits().MaxBaselineSourceBytes {
		return preparedBaseline{}, typedInvalid(
			"source",
			fmt.Sprintf("exceeds %d bytes", catalog.Limits().MaxBaselineSourceBytes),
		)
	}
	if strings.ContainsRune(source, '\x00') {
		return preparedBaseline{}, typedInvalid("source", "contains NUL")
	}
	resource, err := catalog.NormalizeResource(command.Resource)
	if err != nil {
		return preparedBaseline{}, err
	}
	if command.Views < 0 {
		return preparedBaseline{}, typedInvalid("views", "must not be negative")
	}
	fingerprint := postgresBaselineFingerprint(source, resource, command.Views)
	return preparedBaseline{
		source: source, resource: resource, views: command.Views, fingerprint: fingerprint,
	}, nil
}

func importBaselineTx(
	ctx context.Context,
	tx *sql.Tx,
	instanceKey string,
	now time.Time,
	command preparedBaseline,
) (traffic.ImportResult, error) {
	var inserted int
	err := tx.QueryRowContext(ctx, `
INSERT INTO traffic_baselines (
    instance_key, source, resource_kind, resource_id, fingerprint, views
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (instance_key, source, resource_kind, resource_id) DO NOTHING
RETURNING 1`,
		instanceKey, command.source, command.resource.Kind, command.resource.ID,
		command.fingerprint[:], command.views,
	).Scan(&inserted)
	if err != nil && err != sql.ErrNoRows {
		return traffic.ImportResult{}, unavailable("insert baseline", err)
	}
	replay := err == sql.ErrNoRows
	if replay {
		var stored []byte
		if err := tx.QueryRowContext(ctx, `
SELECT fingerprint
FROM traffic_baselines
WHERE instance_key = $1 AND source = $2 AND resource_kind = $3 AND resource_id = $4`,
			instanceKey, command.source, command.resource.Kind, command.resource.ID,
		).Scan(&stored); err != nil {
			return traffic.ImportResult{}, unavailable("load baseline", err)
		}
		if !sameFingerprint(stored, command.fingerprint) {
			return traffic.ImportResult{}, typedConflict(
				"source",
				fmt.Sprintf("%q is reused with a different baseline", command.source),
			)
		}
	} else {
		for _, scope := range []scopeColumns{instanceColumns(), columnsForResource(command.resource)} {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO traffic_totals (
    instance_key, scope_kind, resource_kind, resource_id,
    views, unique_visitor_days, updated_at
)
VALUES ($1, $2, $3, $4, $5, 0, $6)
ON CONFLICT (instance_key, scope_kind, resource_kind, resource_id)
DO UPDATE SET
    views = traffic_totals.views + EXCLUDED.views,
    updated_at = EXCLUDED.updated_at`,
				instanceKey, scope.kind, scope.resourceKind, scope.resourceID,
				command.views, now,
			); err != nil {
				return traffic.ImportResult{}, unavailable("aggregate baseline", err)
			}
		}
	}
	totals, err := readTotals(ctx, tx, instanceKey, columnsForResource(command.resource))
	if err != nil {
		return traffic.ImportResult{}, unavailable("read baseline totals", err)
	}
	return traffic.ImportResult{
		Applied: !replay, Replay: replay, ResourceTotals: totals,
	}, nil
}

func (adapter *Adapter) Prune(ctx context.Context, now time.Time) (traffic.PruneResult, error) {
	if err := ctx.Err(); err != nil {
		return traffic.PruneResult{}, err
	}
	if now.IsZero() {
		return traffic.PruneResult{}, typedInvalid("now", "is required")
	}
	now = now.UTC()
	receiptCutoff := now.Add(-adapter.catalog.Limits().ReceiptRetention)
	markerCutoffDay := adapter.catalog.DayAt(now.Add(-adapter.catalog.Limits().VisitorMarkerRetention))
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return traffic.PruneResult{}, unavailable("begin prune", err)
	}
	defer func() { _ = tx.Rollback() }()
	receipts, err := tx.ExecContext(ctx, `
DELETE FROM traffic_event_receipts
WHERE instance_key = $1 AND received_at < $2`,
		adapter.instanceKey, receiptCutoff,
	)
	if err != nil {
		return traffic.PruneResult{}, unavailable("prune receipts", err)
	}
	markers, err := tx.ExecContext(ctx, `
DELETE FROM traffic_visitor_markers
WHERE instance_key = $1 AND metric_day < $2::date`,
		adapter.instanceKey, markerCutoffDay.String(),
	)
	if err != nil {
		return traffic.PruneResult{}, unavailable("prune visitor markers", err)
	}
	receiptCount, err := receipts.RowsAffected()
	if err != nil {
		return traffic.PruneResult{}, unavailable("count pruned receipts", err)
	}
	markerCount, err := markers.RowsAffected()
	if err != nil {
		return traffic.PruneResult{}, unavailable("count pruned visitor markers", err)
	}
	if err := tx.Commit(); err != nil {
		return traffic.PruneResult{}, unavailable("commit prune", err)
	}
	return traffic.PruneResult{
		ReceiptsRemoved: receiptCount, VisitorMarkersRemoved: markerCount,
	}, nil
}

func (adapter *Adapter) ForgetResource(ctx context.Context, resource traffic.Resource) (traffic.ForgetResult, error) {
	if err := ctx.Err(); err != nil {
		return traffic.ForgetResult{}, err
	}
	resource, err := adapter.catalog.NormalizeResource(resource)
	if err != nil {
		return traffic.ForgetResult{}, err
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return traffic.ForgetResult{}, unavailable("begin resource forget", err)
	}
	defer func() { _ = tx.Rollback() }()
	var result traffic.ForgetResult
	operations := []struct {
		query  string
		target *int64
	}{
		{
			query: `DELETE FROM traffic_totals
WHERE instance_key = $1 AND scope_kind = 'resource' AND resource_kind = $2 AND resource_id = $3`,
			target: &result.TotalsRemoved,
		},
		{
			query: `DELETE FROM traffic_daily
WHERE instance_key = $1 AND scope_kind = 'resource' AND resource_kind = $2 AND resource_id = $3`,
			target: &result.DailyRowsRemoved,
		},
		{
			query: `DELETE FROM traffic_visitor_markers
WHERE instance_key = $1 AND scope_kind = 'resource' AND resource_kind = $2 AND resource_id = $3`,
			target: &result.VisitorMarkersRemoved,
		},
		{
			query: `DELETE FROM traffic_event_receipts
WHERE instance_key = $1 AND resource_kind = $2 AND resource_id = $3`,
			target: &result.ReceiptsRemoved,
		},
		{
			query: `DELETE FROM traffic_baselines
WHERE instance_key = $1 AND resource_kind = $2 AND resource_id = $3`,
			target: &result.BaselinesRemoved,
		},
	}
	for _, operation := range operations {
		executed, err := tx.ExecContext(ctx, operation.query, adapter.instanceKey, resource.Kind, resource.ID)
		if err != nil {
			return traffic.ForgetResult{}, unavailable("forget resource data", err)
		}
		count, err := executed.RowsAffected()
		if err != nil {
			return traffic.ForgetResult{}, unavailable("count forgotten resource data", err)
		}
		*operation.target = count
	}
	if err := tx.Commit(); err != nil {
		return traffic.ForgetResult{}, unavailable("commit resource forget", err)
	}
	return result, nil
}

func postgresBaselineFingerprint(source string, resource traffic.Resource, views int64) [32]byte {
	value := fmt.Sprintf("%s\x00%s\x00%s\x00%d", source, resource.Kind, resource.ID, views)
	return sha256.Sum256([]byte(value))
}
