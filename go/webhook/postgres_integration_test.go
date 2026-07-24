package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/yueli-official/foundation/go/work"
	workpostgres "github.com/yueli-official/foundation/go/work/postgres"
)

type successfulSender struct{}

func (successfulSender) Send(_ context.Context, request OutboundRequest) (SendResult, error) {
	now := time.Now().UTC()
	return SendResult{
		StatusCode: 204, ResponseDigest: digestBytes(nil),
		StartedAt: now, FinishedAt: now,
	}, nil
}

type postgresTestScheduler struct{ work *workpostgres.Adapter }

func (scheduler postgresTestScheduler) Enqueue(ctx context.Context, request DeliveryWork) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	_, err = scheduler.work.Enqueue(ctx, work.Request{
		Kind: WorkKind, Payload: payload, RunAt: request.RunAt, IdempotencyKey: request.Key,
	})
	return err
}

func (scheduler postgresTestScheduler) EnqueueTx(ctx context.Context, tx *sql.Tx, request DeliveryWork) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	_, err = scheduler.work.EnqueueTx(ctx, tx, work.Request{
		Kind: WorkKind, Payload: payload, RunAt: request.RunAt, IdempotencyKey: request.Key,
	})
	return err
}

func TestPostgresPublishTransactionInboundAndDelivery(t *testing.T) {
	db := openWebhookPostgres(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	workCatalog := work.MustCompile(work.Definition{
		Version: work.DefinitionVersion,
		Queues:  []work.QueueDefinition{{Key: "webhook", Concurrency: 1}},
		Kinds:   []work.KindDefinition{WorkDefinition("webhook")},
	})
	workRuntime, err := workpostgres.New(ctx, workCatalog, workpostgres.Options{
		DB: db, InstanceKey: "webhook-work", Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	scheduler := postgresTestScheduler{work: workRuntime}
	secrets := NewMemorySecretStore()
	inboundSecret := []byte("postgres-inbound-secret-32-bytes-long")
	if err := secrets.Create(ctx, "inbound.test", SecretMaterial{
		Revision: "r1", Value: inboundSecret, NotBefore: now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewPostgres(ctx, MustCompile(testDefinition()), PostgresOptions{
		DB: db, InstanceKey: "webhook-runtime", Clock: func() time.Time { return now },
		Scheduler: scheduler, Secrets: secrets, Authorizer: testAuthorizer(now),
	})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := runtime.PutEndpoint(ctx, PutEndpointCommand{ID: "primary", URL: "https://example.com/hooks"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.PutSubscription(ctx, PutSubscriptionCommand{
		ID: "created", EndpointID: endpoint.Endpoint.ID,
		EventTypes: []EventType{"com.yueli.test.created.v1"}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE webhook_test_domain(id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	command := EventCommand{
		Type: "com.yueli.test.created.v1", Data: []byte(`{"id":"rollback"}`),
		OccurredAt: now, IdempotencyKey: "event.rollback",
	}
	rolledBack, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rolledBack.ExecContext(ctx, `INSERT INTO webhook_test_domain(id) VALUES('rollback')`); err != nil {
		t.Fatal(err)
	}
	rolledBackReceipt, err := runtime.PublishTx(ctx, rolledBack, command)
	if err != nil {
		_ = rolledBack.Rollback()
		t.Fatalf("%v: %v", err, errors.Unwrap(err))
	}
	if err := rolledBack.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Event(ctx, rolledBackReceipt.EventID); !IsCode(err, ErrorNotFound) {
		t.Fatalf("rolled-back event err=%v", err)
	}
	command.Data, command.IdempotencyKey = []byte(`{"id":"commit"}`), "event.commit"
	committed, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := committed.ExecContext(ctx, `INSERT INTO webhook_test_domain(id) VALUES('commit')`); err != nil {
		t.Fatal(err)
	}
	receipt, err := runtime.PublishTx(ctx, committed, command)
	if err != nil {
		t.Fatal(err)
	}
	if err := committed.Commit(); err != nil {
		t.Fatal(err)
	}
	if receipt.Deliveries != 1 {
		t.Fatalf("receipt=%+v", receipt)
	}
	page, err := runtime.ListDeliveries(ctx, DeliveryQuery{Limit: 10})
	if err != nil || len(page.Deliveries) != 1 {
		t.Fatalf("deliveries=%+v err=%v", page, err)
	}
	driver := &DeliveryDriver{
		Backend: runtime, Secrets: secrets,
		Authorizer: NetworkAuthorizer{
			Resolver: &scriptedResolver{answers: [][]netip.Addr{{netip.MustParseAddr("93.184.216.34")}}},
			Policy:   PublicNetworkPolicy(), Clock: func() time.Time { return now },
		},
		Sender: successfulSender{}, Retry: runtime.catalog.retry, Limits: runtime.catalog.limits,
		Clock: func() time.Time { return now },
	}
	delivered, err := driver.Advance(ctx, page.Deliveries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if delivered.State != DeliveryDelivered {
		t.Fatalf("delivered=%+v", delivered)
	}
	attempts, err := runtime.Attempts(ctx, delivered.ID)
	if err != nil || len(attempts) != 1 || attempts[0].Outcome != AttemptSucceeded {
		t.Fatalf("attempts=%+v err=%v", attempts, err)
	}
	body, err := encodeCloudEvent(runtime.catalog, "incoming-1", EventCommand{
		Type: "com.yueli.test.created.v1", Data: []byte(`{"id":"incoming"}`), OccurredAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{}
	headers.Set(HeaderWebhookID, "incoming-1")
	headers.Set(HeaderWebhookTimestamp, strconv.FormatInt(now.Unix(), 10))
	headers.Set(HeaderWebhookSignature, SignV1("incoming-1", now, body, inboundSecret))
	verified, err := runtime.Verify(ctx, "test.sender", IncomingMessage{Headers: headers, Body: body, ReceivedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	inboundTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.AcceptTx(ctx, inboundTx, verified); err != nil {
		t.Fatal(err)
	}
	if _, err := inboundTx.ExecContext(ctx, `INSERT INTO webhook_test_domain(id) VALUES('incoming')`); err != nil {
		t.Fatal(err)
	}
	if err := inboundTx.Rollback(); err != nil {
		t.Fatal(err)
	}
	accepted, err := runtime.Accept(ctx, verified)
	if err != nil || !accepted.FirstSeen {
		t.Fatalf("accept after rollback=%+v err=%v", accepted, err)
	}
}

var webhookSchemaCounter atomic.Uint64

func openWebhookPostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("WEBHOOK_POSTGRES_DSN")
	if dsn == "" {
		dsn = os.Getenv("WORK_POSTGRES_DSN")
	}
	if dsn == "" {
		t.Skip("WEBHOOK_POSTGRES_DSN is not configured")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schemaName := fmt.Sprintf("webhook_test_%d_%d", time.Now().UnixNano(), webhookSchemaCounter.Add(1))
	if _, err := admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schemaName)); err != nil {
		_ = admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	scoped, err := webhookSearchPath(dsn, schemaName)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("postgres", scoped)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(16)
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	workMigration, err := workpostgres.Schema(workpostgres.CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(workMigration.UpSQL); err != nil {
		t.Fatalf("work schema up: %v", err)
	}
	webhookMigration, err := Schema(CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(webhookMigration.UpSQL); err != nil {
		t.Fatalf("webhook schema up: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(webhookMigration.DownSQL)
		_, _ = db.Exec(workMigration.DownSQL)
		_ = db.Close()
		_, _ = admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schemaName) + " CASCADE")
		_ = admin.Close()
	})
	return db
}

func webhookSearchPath(dsn, schema string) (string, error) {
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
