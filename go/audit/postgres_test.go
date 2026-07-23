package audit_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"

	"github.com/yueli-official/foundation/go/audit"
	"github.com/yueli-official/foundation/go/audit/audittest"
)

func TestPostgresSchemaCoversJournalLifecycle(t *testing.T) {
	schema := audit.PostgresSchemaUp()
	for _, relation := range []string{
		"audit_instances", "audit_action_definitions", "audit_events",
		"audit_event_receipts", "audit_legal_holds", "audit_retention_receipts",
		"audit_mirror_outbox",
	} {
		if !strings.Contains(schema, "CREATE TABLE "+relation) {
			t.Fatalf("schema does not create %s", relation)
		}
	}
	if !strings.Contains(audit.PostgresSchemaDown(), "DROP TABLE IF EXISTS audit_instances") {
		t.Fatal("down migration does not remove instance metadata")
	}
}

func TestWritePostgresMigrationIsImmutable(t *testing.T) {
	directory := t.TempDir()
	first, err := audit.WritePostgresMigration(directory, "0018_audit_v1", audit.PostgresSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	second, err := audit.WritePostgresMigration(directory, "0018_audit_v1", audit.PostgresSchemaVersion)
	if err != nil || second != first {
		t.Fatalf("repeat = %#v, %v; want %#v", second, err, first)
	}
	if err := os.WriteFile(first.UpPath, []byte("drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := audit.WritePostgresMigration(directory, "0018_audit_v1", audit.PostgresSchemaVersion); err == nil {
		t.Fatal("drifted migration was overwritten")
	}
}

func TestPostgresConformance(t *testing.T) {
	database := openAuditPostgres(t)
	var counter atomic.Uint64
	audittest.Run(t, func(t *testing.T, catalog *audit.Catalog, clock audit.Clock) audit.Module {
		adapter, err := audit.NewPostgres(context.Background(), catalog, audit.PostgresOptions{
			DB: database, InstanceKey: fmt.Sprintf("audit-conformance-%d", counter.Add(1)),
			Source: audit.Source{Service: "audit-test", Instance: "test"}, Clock: clock,
		})
		if err != nil {
			t.Fatal(err)
		}
		return postgresTestModule{Postgres: adapter, database: database}
	})
}

func TestPostgresCallerTransactionRollbackLeavesNoAuditGap(t *testing.T) {
	database := openAuditPostgres(t)
	catalog, contract := retentionCatalog(t)
	adapter, err := audit.NewPostgres(context.Background(), catalog, audit.PostgresOptions{
		DB: database, InstanceKey: "transaction-rollback",
		Source: audit.Source{Service: "docs", Instance: "docs-main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := database.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	appender, err := adapter.Bind(tx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := audit.Record(context.Background(), appender, contract, retentionAttempt("rolled-back", 1)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	tx, err = database.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	appender, _ = adapter.Bind(tx)
	event, err := audit.Record(context.Background(), appender, contract, retentionAttempt("committed", 2))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if event.Sequence != 1 || event.PreviousDigest != "" {
		t.Fatalf("event after rollback = %#v", event)
	}
}

func TestPostgresRetentionAndCommittedMirror(t *testing.T) {
	database := openAuditPostgres(t)
	now := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	catalog, contract := retentionCatalog(t)
	adapter, err := audit.NewPostgres(context.Background(), catalog, audit.PostgresOptions{
		DB: database, InstanceKey: "retention-mirror",
		Source: audit.Source{Service: "docs", Instance: "docs-main"},
		Clock:  fixedRetentionClock{value: now}, EnableMirrorOutbox: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	module := postgresTestModule{Postgres: adapter, database: database}
	for index := range 2 {
		if _, err := audit.Record(
			context.Background(), module, contract,
			retentionAttempt(audit.EventID(fmt.Sprintf("event-%d", index+1)), uint64(index+1)),
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := adapter.PlaceHold(context.Background(), audit.PlaceHoldCommand{
		ID: "case-1", Reason: "legal.request", Actor: audit.Actor{Kind: audit.ActorUser, ID: "admin-1"},
		Selection: audit.HoldSelection{EventIDs: []audit.EventID{"event-2"}},
	}); err != nil {
		t.Fatal(err)
	}
	sink := &memoryArchiveSink{}
	receipt, err := adapter.RunRetention(context.Background(), audit.RetentionCommand{
		ID: "retention-1", AsOf: now.Add(31 * 24 * time.Hour),
		Actor: audit.Actor{Kind: audit.ActorSystem, ID: "retention-worker"}, Archive: sink,
	})
	if err != nil || receipt.Deleted != 1 {
		t.Fatalf("RunRetention() = %#v, %v", receipt, err)
	}
	mirror := &captureMirror{}
	dispatched, err := adapter.DispatchMirror(context.Background(), mirror, audit.MirrorDispatchOptions{
		Clock: fixedRetentionClock{value: now.Add(time.Minute)},
	})
	if err != nil || dispatched.Delivered != 1 || len(mirror.events) != 1 || mirror.events[0].Event.ID != "event-2" {
		t.Fatalf("DispatchMirror() = %#v, events %#v, %v", dispatched, mirror.events, err)
	}
	backlog, err := adapter.MirrorBacklog(context.Background())
	if err != nil || backlog.Pending != 0 {
		t.Fatalf("MirrorBacklog() = %#v, %v", backlog, err)
	}
	verified, err := adapter.Verify(context.Background(), audit.VerifyRequest{})
	if err != nil || !verified.Valid || verified.Count != 1 {
		t.Fatalf("Verify() = %#v, %v", verified, err)
	}
	if _, err := audit.Record(
		context.Background(), module, contract, retentionAttempt("event-1", 1),
	); !audit.IsKind(err, audit.ErrorIdempotencyConflict) {
		t.Fatalf("purged ID replay error = %v", err)
	}
}

type postgresTestModule struct {
	*audit.Postgres
	database *sql.DB
}

type captureMirror struct{ events []audit.CommittedEvent }

func (mirror *captureMirror) Publish(_ context.Context, events []audit.CommittedEvent) error {
	mirror.events = append(mirror.events, events...)
	return nil
}

func (module postgresTestModule) Append(ctx context.Context, command audit.Command) (audit.Event, error) {
	events, err := module.AppendBatch(ctx, []audit.Command{command})
	if err != nil {
		return audit.Event{}, err
	}
	return events[0], nil
}

func (module postgresTestModule) AppendBatch(ctx context.Context, commands []audit.Command) ([]audit.Event, error) {
	tx, err := module.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	appender, err := module.Postgres.Bind(tx)
	if err != nil {
		return nil, err
	}
	events, err := appender.AppendBatch(ctx, commands)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return events, nil
}

func openAuditPostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("AUDIT_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("AUDIT_POSTGRES_DSN is not configured")
	}
	database, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	schema := fmt.Sprintf("audit_test_%d", time.Now().UnixNano())
	if _, err := database.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema)); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if _, err := database.Exec("SET search_path TO " + pq.QuoteIdentifier(schema)); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if _, err := database.Exec(audit.PostgresSchemaUp()); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(audit.PostgresSchemaDown())
		_, _ = database.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema))
		_ = database.Close()
	})
	return database
}
