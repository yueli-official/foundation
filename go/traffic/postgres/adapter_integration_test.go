package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/yueli-official/foundation/go/traffic"
	"github.com/yueli-official/foundation/go/traffic/postgres"
	"github.com/yueli-official/foundation/go/traffic/traffictest"
)

func TestPostgresAdapterConformance(t *testing.T) {
	database := openPostgresTestDatabase(t)
	var instances atomic.Uint64
	traffictest.Run(t, func(t *testing.T, catalog *traffic.Catalog, clock *traffictest.Clock) traffic.Module {
		t.Helper()
		adapter, err := postgres.New(context.Background(), catalog, postgres.Options{
			DB: database, InstanceKey: fmt.Sprintf("conformance-%d", instances.Add(1)),
			Clock: clock.Now, Secret: []byte("postgres-conformance-secret-32-bytes"),
		})
		if err != nil {
			t.Fatal(err)
		}
		return adapter
	})
}

func TestPostgresRestartRetainsSecretReceiptsAndBaselines(t *testing.T) {
	database := openPostgresTestDatabase(t)
	catalog := traffic.MustCompile(testDefinition())
	clock := traffictest.NewClock(time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC))
	ctx := context.Background()
	first, err := postgres.New(ctx, catalog, postgres.Options{
		DB: database, InstanceKey: "restart", Clock: clock.Now,
		Secret: []byte("first-secret-at-least-thirty-two-bytes"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.InstanceWasCreated() {
		t.Fatal("first adapter must report a created instance")
	}
	token, err := first.TokenizeVisitor(ctx, clock.Now(), []byte("visitor-a"))
	if err != nil {
		t.Fatal(err)
	}
	observation := traffic.Observation{
		EventID: "restart-event-0001", Resource: traffic.Resource{Kind: "post", ID: "post-a"},
		OccurredAt: clock.Now(), Class: traffic.VisitHuman,
		HasVisitor: true, VisitorToken: token,
	}
	recorded, err := first.Record(ctx, observation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.ImportBaseline(ctx, traffic.BaselineImport{
		Source: "post_stats", Resource: observation.Resource, Views: 20,
	}); err != nil {
		t.Fatal(err)
	}

	restarted, err := postgres.New(ctx, catalog, postgres.Options{
		DB: database, InstanceKey: "restart", Clock: clock.Now,
		Secret: []byte("different-secret-is-ignored-on-restart"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.InstanceWasCreated() {
		t.Fatal("restart must not report a new instance")
	}
	restartedToken, err := restarted.TokenizeVisitor(ctx, clock.Now(), []byte("visitor-a"))
	if err != nil {
		t.Fatal(err)
	}
	if restartedToken != token {
		t.Fatal("persisted instance secret did not survive restart")
	}
	replayed, err := restarted.Record(ctx, observation)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replay || replayed.ResourceTotals.Views != 21 || recorded.ResourceTotals.Views != 1 {
		t.Fatalf("unexpected restart replay: before=%+v after=%+v", recorded, replayed)
	}
	baseline, err := restarted.ImportBaseline(ctx, traffic.BaselineImport{
		Source: "post_stats", Resource: observation.Resource, Views: 20,
	})
	if err != nil || !baseline.Replay || baseline.ResourceTotals.Views != 21 {
		t.Fatalf("baseline did not replay across restart: %+v err=%v", baseline, err)
	}
}

func TestPostgresCatalogMismatchFailsClosed(t *testing.T) {
	database := openPostgresTestDatabase(t)
	first := traffic.MustCompile(testDefinition())
	if _, err := postgres.New(context.Background(), first, postgres.Options{
		DB: database, InstanceKey: "mismatch",
	}); err != nil {
		t.Fatal(err)
	}
	changed := traffic.MustCompile(traffic.Definition{
		Version: traffic.DefinitionVersion, TimeZone: "UTC",
		ResourceKinds: []traffic.ResourceKindDefinition{{Key: "post"}, {Key: "asset"}},
	})
	if _, err := postgres.New(context.Background(), changed, postgres.Options{
		DB: database, InstanceKey: "mismatch",
	}); !traffic.IsKind(err, traffic.ErrorConflict) {
		t.Fatalf("catalog mismatch must fail closed, got %v", err)
	}
}

func TestPostgresInitialBaselinesAreAtomicAndCreationOnly(t *testing.T) {
	database := openPostgresTestDatabase(t)
	catalog := traffic.MustCompile(testDefinition())
	ctx := context.Background()
	resource := traffic.Resource{Kind: "post", ID: "legacy-post"}

	if _, err := postgres.New(ctx, catalog, postgres.Options{
		DB: database, InstanceKey: "initial-baseline-invalid",
		InitialBaselines: []traffic.BaselineImport{
			{Source: "post_stats", Resource: resource, Views: 12},
			{Source: "", Resource: resource, Views: 1},
		},
	}); !traffic.IsKind(err, traffic.ErrorInvalidInput) {
		t.Fatalf("invalid baseline must abort creation, got %v", err)
	}
	retry, err := postgres.New(ctx, catalog, postgres.Options{
		DB: database, InstanceKey: "initial-baseline-invalid",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !retry.InstanceWasCreated() {
		t.Fatal("failed initial baseline transaction left an instance behind")
	}

	first, err := postgres.New(ctx, catalog, postgres.Options{
		DB: database, InstanceKey: "initial-baseline-success",
		InitialBaselines: []traffic.BaselineImport{
			{Source: "post_stats", Resource: resource, Views: 12},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	totals, err := first.Totals(ctx, []traffic.Resource{resource})
	if err != nil || len(totals) != 1 || totals[0].Totals.Views != 12 {
		t.Fatalf("initial baseline was not committed: totals=%+v err=%v", totals, err)
	}
	restarted, err := postgres.New(ctx, catalog, postgres.Options{
		DB: database, InstanceKey: "initial-baseline-success",
		InitialBaselines: []traffic.BaselineImport{
			{Source: "post_stats", Resource: resource, Views: 999},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	totals, err = restarted.Totals(ctx, []traffic.Resource{resource})
	if err != nil || totals[0].Totals.Views != 12 {
		t.Fatalf("restart re-imported initial baseline: totals=%+v err=%v", totals, err)
	}
}

func TestPostgresSchemaDownRemovesAllTables(t *testing.T) {
	database := openPostgresTestDatabase(t)
	migration, err := postgres.Schema(postgres.CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(migration.DownSQL); err != nil {
		t.Fatal(err)
	}
	var relation sql.NullString
	if err := database.QueryRow(`SELECT to_regclass('traffic_instances')`).Scan(&relation); err != nil {
		t.Fatal(err)
	}
	if relation.Valid {
		t.Fatalf("traffic_instances still exists as %q", relation.String)
	}
}

func TestPostgresScaleQueriesStayInsideBudgetAndUseTopIndex(t *testing.T) {
	database := openPostgresTestDatabase(t)
	catalog := traffic.MustCompile(testDefinition())
	ctx := context.Background()
	module, err := postgres.New(ctx, catalog, postgres.Options{
		DB: database, InstanceKey: "scale-query",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
INSERT INTO traffic_totals (
    instance_key, scope_kind, resource_kind, resource_id,
    views, unique_visitor_days, updated_at
)
SELECT 'scale-query', 'resource', 'post',
       'post-' || lpad(resource_no::text, 6, '0'),
       resource_no * 7, resource_no * 3, NOW()
FROM generate_series(1, 20000) AS resource_no`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
INSERT INTO traffic_daily (
    instance_key, metric_day, scope_kind, resource_kind, resource_id,
    views, unique_visitor_days, updated_at
)
SELECT 'scale-query', metric_day::date, 'resource', 'post',
       'post-' || lpad(resource_no::text, 6, '0'),
       (resource_no % 17) + 1, (resource_no % 11) + 1, NOW()
FROM generate_series(1, 2000) AS resource_no
CROSS JOIN generate_series(
    DATE '2026-01-01', DATE '2026-03-31', INTERVAL '1 day'
) AS metric_day`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("ANALYZE traffic_totals; ANALYZE traffic_daily"); err != nil {
		t.Fatal(err)
	}

	var plan strings.Builder
	rows, err := database.Query(`
EXPLAIN (COSTS OFF)
SELECT resource_kind, resource_id, views, unique_visitor_days
FROM traffic_totals
WHERE instance_key = 'scale-query'
  AND scope_kind = 'resource'
  AND resource_kind = 'post'
ORDER BY views DESC, resource_id
LIMIT 20`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "traffic_totals_top_views_idx") {
		t.Fatalf("all-time top query did not use its covering order index:\n%s", plan.String())
	}

	budgetContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := module.Top(budgetContext, traffic.TopQuery{
		ResourceKind: "post", Limit: 100,
		Range: &traffic.DateRange{
			From: traffic.MustParseDay("2026-03-01"),
			To:   traffic.MustParseDay("2026-04-01"),
		},
	}); err != nil {
		t.Fatalf("range top exceeded scale query budget: %v", err)
	}
	if _, err := module.Series(budgetContext, traffic.SeriesQuery{
		Scope: traffic.ResourceScope(traffic.Resource{Kind: "post", ID: "post-001000"}),
		Range: traffic.DateRange{
			From: traffic.MustParseDay("2026-01-01"),
			To:   traffic.MustParseDay("2026-04-01"),
		},
	}); err != nil {
		t.Fatalf("resource series exceeded scale query budget: %v", err)
	}
	resources := make([]traffic.Resource, 1000)
	for index := range resources {
		resources[index] = traffic.Resource{
			Kind: "post", ID: fmt.Sprintf("post-%06d", index+1),
		}
	}
	if _, err := module.Totals(budgetContext, resources); err != nil {
		t.Fatalf("batch totals exceeded scale query budget: %v", err)
	}
}

func openPostgresTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TRAFFIC_POSTGRES_DSN")
	if dsn == "" {
		dsn = os.Getenv("AUTHORIZATION_POSTGRES_DSN")
	}
	if dsn == "" {
		t.Skip("TRAFFIC_POSTGRES_DSN is not configured")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schemaName := fmt.Sprintf("traffic_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schemaName)); err != nil {
		_ = admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	scopedDSN, err := withSearchPath(dsn, schemaName)
	if err != nil {
		_, _ = admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schemaName) + " CASCADE")
		_ = admin.Close()
		t.Fatal(err)
	}
	database, err := sql.Open("postgres", scopedDSN)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(32)
	if err := database.Ping(); err != nil {
		t.Fatal(err)
	}
	migration, err := postgres.Schema(postgres.CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(migration.UpSQL); err != nil {
		t.Fatalf("schema up: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(migration.DownSQL)
		_ = database.Close()
		_, _ = admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schemaName) + " CASCADE")
		_ = admin.Close()
	})
	return database
}

func withSearchPath(dsn, schema string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "postgres" || parsed.Scheme == "postgresql" {
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		return parsed.String(), nil
	}
	return dsn + " search_path=" + pq.QuoteIdentifier(schema), nil
}

func testDefinition() traffic.Definition {
	return traffic.Definition{
		Version: traffic.DefinitionVersion, TimeZone: "Asia/Shanghai",
		ResourceKinds: []traffic.ResourceKindDefinition{{Key: "post"}},
	}
}
