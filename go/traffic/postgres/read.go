package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/yueli-official/foundation/go/traffic"
)

func (adapter *Adapter) Summary(ctx context.Context, query traffic.SummaryQuery) (traffic.Summary, error) {
	if err := ctx.Err(); err != nil {
		return traffic.Summary{}, err
	}
	scope, err := adapter.catalog.NormalizeScope(query.Scope)
	if err != nil {
		return traffic.Summary{}, err
	}
	columns := columnsForScope(scope)
	var totals traffic.Totals
	var normalizedRange *traffic.DateRange
	if query.Range == nil {
		totals, err = readTotals(ctx, adapter.db, adapter.instanceKey, columns)
		if err != nil {
			return traffic.Summary{}, unavailable("read summary", err)
		}
	} else {
		value, rangeErr := adapter.catalog.NormalizeRange(*query.Range)
		if rangeErr != nil {
			return traffic.Summary{}, rangeErr
		}
		normalizedRange = &value
		err = adapter.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(views), 0), COALESCE(SUM(unique_visitor_days), 0)
FROM traffic_daily
WHERE instance_key = $1 AND scope_kind = $2
  AND resource_kind = $3 AND resource_id = $4
  AND metric_day >= $5::date AND metric_day < $6::date`,
			adapter.instanceKey, columns.kind, columns.resourceKind, columns.resourceID,
			value.From.String(), value.To.String(),
		).Scan(&totals.Views, &totals.UniqueVisitorDays)
		if err != nil {
			return traffic.Summary{}, unavailable("read range summary", err)
		}
	}
	return traffic.Summary{Scope: scope, Range: normalizedRange, Totals: totals}, nil
}

func (adapter *Adapter) Series(ctx context.Context, query traffic.SeriesQuery) ([]traffic.SeriesPoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	scope, err := adapter.catalog.NormalizeScope(query.Scope)
	if err != nil {
		return nil, err
	}
	dateRange, err := adapter.catalog.NormalizeRange(query.Range)
	if err != nil {
		return nil, err
	}
	columns := columnsForScope(scope)
	rows, err := adapter.db.QueryContext(ctx, `
SELECT days.metric_day::date,
       COALESCE(daily.views, 0),
       COALESCE(daily.unique_visitor_days, 0)
FROM generate_series(
    $2::date,
    $3::date - INTERVAL '1 day',
    INTERVAL '1 day'
) AS days(metric_day)
LEFT JOIN traffic_daily daily
  ON daily.instance_key = $1
 AND daily.metric_day = days.metric_day::date
 AND daily.scope_kind = $4
 AND daily.resource_kind = $5
 AND daily.resource_id = $6
ORDER BY days.metric_day`,
		adapter.instanceKey, dateRange.From.String(), dateRange.To.String(),
		columns.kind, columns.resourceKind, columns.resourceID,
	)
	if err != nil {
		return nil, unavailable("read series", err)
	}
	defer rows.Close()
	result := make([]traffic.SeriesPoint, 0)
	for rows.Next() {
		var (
			value  time.Time
			totals traffic.Totals
		)
		if err := rows.Scan(&value, &totals.Views, &totals.UniqueVisitorDays); err != nil {
			return nil, unavailable("scan series", err)
		}
		day, err := traffic.ParseDay(value.Format(time.DateOnly))
		if err != nil {
			return nil, unavailable("decode series day", err)
		}
		result = append(result, traffic.SeriesPoint{Day: day, Totals: totals})
	}
	if err := rows.Err(); err != nil {
		return nil, unavailable("iterate series", err)
	}
	return result, nil
}

func (adapter *Adapter) Top(ctx context.Context, query traffic.TopQuery) ([]traffic.TopEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resource, err := adapter.catalog.NormalizeResource(traffic.Resource{Kind: query.ResourceKind, ID: "_validation_"})
	if err != nil {
		return nil, &traffic.Error{Kind: traffic.ErrorInvalidInput, Field: "resource_kind", Message: "is not registered"}
	}
	kind := resource.Kind
	metric := query.Metric
	if metric == "" {
		metric = traffic.RankViews
	}
	orderColumn := "views"
	if metric == traffic.RankUniqueVisitorDays {
		orderColumn = "unique_visitor_days"
	} else if metric != traffic.RankViews {
		return nil, typedInvalid("metric", "is unknown")
	}
	limit := query.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 1000 {
		return nil, typedInvalid("limit", "must be between 1 and 1000")
	}
	var rows *sql.Rows
	if query.Range == nil {
		rows, err = adapter.db.QueryContext(ctx, `
SELECT resource_kind, resource_id, views, unique_visitor_days
FROM traffic_totals
WHERE instance_key = $1 AND scope_kind = 'resource' AND resource_kind = $2
ORDER BY `+orderColumn+` DESC, resource_id
LIMIT $3`, adapter.instanceKey, kind, limit)
	} else {
		dateRange, rangeErr := adapter.catalog.NormalizeRange(*query.Range)
		if rangeErr != nil {
			return nil, rangeErr
		}
		rows, err = adapter.db.QueryContext(ctx, `
SELECT resource_kind, resource_id,
       COALESCE(SUM(views), 0)::bigint AS views,
       COALESCE(SUM(unique_visitor_days), 0)::bigint AS unique_visitor_days
FROM traffic_daily
WHERE instance_key = $1 AND scope_kind = 'resource' AND resource_kind = $2
  AND metric_day >= $3::date AND metric_day < $4::date
GROUP BY resource_kind, resource_id
ORDER BY `+orderColumn+` DESC, resource_id
LIMIT $5`,
			adapter.instanceKey, kind, dateRange.From.String(), dateRange.To.String(), limit,
		)
	}
	if err != nil {
		return nil, unavailable("read top resources", err)
	}
	defer rows.Close()
	result := make([]traffic.TopEntry, 0)
	for rows.Next() {
		var entry traffic.TopEntry
		if err := rows.Scan(
			&entry.Resource.Kind, &entry.Resource.ID,
			&entry.Totals.Views, &entry.Totals.UniqueVisitorDays,
		); err != nil {
			return nil, unavailable("scan top resources", err)
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, unavailable("iterate top resources", err)
	}
	return result, nil
}

func (adapter *Adapter) Totals(ctx context.Context, resources []traffic.Resource) ([]traffic.ResourceTotals, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(resources) == 0 {
		return []traffic.ResourceTotals{}, nil
	}
	if len(resources) > 1000 {
		return nil, typedInvalid("resources", "exceeds 1000 items")
	}
	normalized := make([]traffic.Resource, len(resources))
	args := make([]any, 1, 1+len(resources)*3)
	args[0] = adapter.instanceKey
	values := make([]string, 0, len(resources))
	position := 2
	for index, resource := range resources {
		value, err := adapter.catalog.NormalizeResource(resource)
		if err != nil {
			return nil, indexedError(err, "resources", index)
		}
		normalized[index] = value
		values = append(values, fmt.Sprintf("($%d::integer, $%d::text, $%d::text)", position, position+1, position+2))
		args = append(args, index, value.Kind, value.ID)
		position += 3
	}
	query := `
WITH requested(ordinal, resource_kind, resource_id) AS (
    VALUES ` + strings.Join(values, ",") + `
)
SELECT requested.ordinal, requested.resource_kind, requested.resource_id,
       COALESCE(totals.views, 0), COALESCE(totals.unique_visitor_days, 0)
FROM requested
LEFT JOIN traffic_totals totals
  ON totals.instance_key = $1
 AND totals.scope_kind = 'resource'
 AND totals.resource_kind = requested.resource_kind
 AND totals.resource_id = requested.resource_id
ORDER BY requested.ordinal`
	rows, err := adapter.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, unavailable("read resource totals", err)
	}
	defer rows.Close()
	result := make([]traffic.ResourceTotals, len(normalized))
	for rows.Next() {
		var (
			index int
			item  traffic.ResourceTotals
		)
		if err := rows.Scan(
			&index, &item.Resource.Kind, &item.Resource.ID,
			&item.Totals.Views, &item.Totals.UniqueVisitorDays,
		); err != nil {
			return nil, unavailable("scan resource totals", err)
		}
		if index < 0 || index >= len(result) {
			return nil, unavailable("decode resource total ordinal", fmt.Errorf("out of range ordinal %d", index))
		}
		result[index] = item
	}
	if err := rows.Err(); err != nil {
		return nil, unavailable("iterate resource totals", err)
	}
	return result, nil
}
