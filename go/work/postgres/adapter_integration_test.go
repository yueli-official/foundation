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
	"github.com/yueli-official/foundation/go/work"
	"github.com/yueli-official/foundation/go/work/postgres"
	"github.com/yueli-official/foundation/go/work/worktest"
)

func TestPostgresAdapterConformance(t *testing.T) {
	database := openPostgresTestDatabase(t)
	var instances atomic.Uint64
	worktest.Run(t, func(t *testing.T, catalog *work.Catalog, clock *worktest.Clock) work.Backend {
		t.Helper()
		adapter, err := postgres.New(context.Background(), catalog, postgres.Options{
			DB: database, InstanceKey: fmt.Sprintf("conformance-%d", instances.Add(1)),
			Clock: clock.Now,
		})
		if err != nil {
			t.Fatal(err)
		}
		return adapter
	})
}

func TestEnqueueTxCommitsAndRollsBackWithDomainData(t *testing.T) {
	database := openPostgresTestDatabase(t)
	catalog := testCatalog()
	ctx := context.Background()
	adapter, err := postgres.New(ctx, catalog, postgres.Options{
		DB: database, InstanceKey: "transactional",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE domain_events (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rolledBack.ExecContext(ctx, `INSERT INTO domain_events (id) VALUES ('rollback')`); err != nil {
		t.Fatal(err)
	}
	rolledBackJob, err := adapter.EnqueueTx(ctx, rolledBack, work.Request{
		Kind: "mail.send", IdempotencyKey: "tx:rollback:000001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rolledBack.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Get(ctx, rolledBackJob.Job.ID); !work.IsKind(err, work.ErrorNotFound) {
		t.Fatalf("rolled back job remained: %v", err)
	}

	committed, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := committed.ExecContext(ctx, `INSERT INTO domain_events (id) VALUES ('commit')`); err != nil {
		t.Fatal(err)
	}
	committedJob, err := adapter.EnqueueTx(ctx, committed, work.Request{
		Kind: "mail.send", IdempotencyKey: "tx:commit:0000001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := committed.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Get(ctx, committedJob.Job.ID); err != nil {
		t.Fatalf("committed job is absent: %v", err)
	}
	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM domain_events WHERE id = 'commit'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("domain mutation did not commit with the job")
	}
}

func TestPostgresRestartAndCatalogMismatch(t *testing.T) {
	database := openPostgresTestDatabase(t)
	catalog := testCatalog()
	ctx := context.Background()
	first, err := postgres.New(ctx, catalog, postgres.Options{
		DB: database, InstanceKey: "restart",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.InstanceWasCreated() {
		t.Fatal("first adapter must report creation")
	}
	enqueued, err := first.Enqueue(ctx, work.Request{
		Kind: "mail.send", IdempotencyKey: "restart:0000000001",
	})
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := postgres.New(ctx, catalog, postgres.Options{
		DB: database, InstanceKey: "restart",
	})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.InstanceWasCreated() {
		t.Fatal("restart reported a new instance")
	}
	replay, err := restarted.Enqueue(ctx, work.Request{
		Kind: "mail.send", IdempotencyKey: "restart:0000000001",
	})
	if err != nil || !replay.Replay || replay.Job.ID != enqueued.Job.ID {
		t.Fatalf("durable replay failed: %+v err=%v", replay, err)
	}
	changed := work.MustCompile(work.Definition{
		Version: work.DefinitionVersion,
		Queues:  []work.QueueDefinition{{Key: "delivery", Concurrency: 2}},
		Kinds:   []work.KindDefinition{{Key: "mail.send", Queue: "delivery"}},
	})
	if _, err := postgres.New(ctx, changed, postgres.Options{
		DB: database, InstanceKey: "restart",
	}); !work.IsKind(err, work.ErrorConflict) {
		t.Fatalf("catalog mismatch must fail closed, got %v", err)
	}
}

func TestPostgresScaleClaimUsesDueIndex(t *testing.T) {
	database := openPostgresTestDatabase(t)
	ctx := context.Background()
	adapter, err := postgres.New(ctx, testCatalog(), postgres.Options{
		DB: database, InstanceKey: "scale",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
INSERT INTO work_jobs (
    instance_key, job_id, kind, queue, status, payload, metadata,
    attempt, max_attempts, priority, run_at, fingerprint,
    idempotency_key, created_at, updated_at
)
SELECT
    'scale',
    md5('work-scale-' || item)::uuid,
    'mail.send',
    'delivery',
    'queued',
    '{}'::jsonb,
    '{}'::jsonb,
    0,
    3,
    item % 11,
    NOW() - (item * INTERVAL '1 millisecond'),
    decode(repeat('00', 32), 'hex'),
    'scale:' || item,
    NOW() - (item * INTERVAL '1 millisecond'),
    NOW()
FROM generate_series(1, 20000) AS item`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`ANALYZE work_jobs`); err != nil {
		t.Fatal(err)
	}
	rows, err := database.Query(`
EXPLAIN (COSTS OFF)
SELECT job_id
FROM work_jobs
WHERE instance_key = 'scale'
  AND queue = 'delivery'
  AND status IN ('queued', 'retrying')
  AND run_at <= NOW()
  AND attempt < max_attempts
ORDER BY priority DESC, run_at, created_at, job_id
FOR UPDATE SKIP LOCKED
LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	var plan strings.Builder
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
	if !strings.Contains(plan.String(), "work_jobs_due_claim_idx") {
		t.Fatalf("claim query did not use the due index:\n%s", plan.String())
	}
	budget, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	claimed, err := adapter.Claim(budget, work.ClaimRequest{
		Queue: "delivery", WorkerID: "scale-worker", LeaseDuration: 30 * time.Second,
	})
	if err != nil || !claimed.Found {
		t.Fatalf("scale claim exceeded budget or found no job: found=%v err=%v", claimed.Found, err)
	}
	stats, err := adapter.Stats(budget)
	if err != nil || stats.Due != 19999 || stats.Running != 1 {
		t.Fatalf("unexpected scale stats: %+v err=%v", stats, err)
	}
}

func openPostgresTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("WORK_POSTGRES_DSN")
	if dsn == "" {
		dsn = os.Getenv("TRAFFIC_POSTGRES_DSN")
	}
	if dsn == "" {
		t.Skip("WORK_POSTGRES_DSN is not configured")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schemaName := fmt.Sprintf("work_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schemaName)); err != nil {
		_ = admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	scopedDSN, err := withSearchPath(dsn, schemaName)
	if err != nil {
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

func testCatalog() *work.Catalog {
	return work.MustCompile(work.Definition{
		Version: work.DefinitionVersion,
		Queues:  []work.QueueDefinition{{Key: "delivery", Concurrency: 1}},
		Kinds: []work.KindDefinition{
			{Key: "mail.send", Queue: "delivery", DefaultAttempts: 3, MaxAttempts: 10},
		},
	})
}
