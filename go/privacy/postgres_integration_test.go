package privacy_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/yueli-official/foundation/go/privacy"
	"github.com/yueli-official/foundation/go/privacy/privacytest"
)

func TestPostgresRuntimeConformance(t *testing.T) {
	db := privacyPostgresDatabase(t)
	privacytest.RunRuntime(t, func(t *testing.T, catalog *privacy.Catalog, clock func() time.Time) privacy.Runtime {
		t.Helper()
		runtime, err := privacy.NewPostgresRuntime(context.Background(), catalog, privacy.PostgresOptions{
			DB: db, InstanceKey: "runtime-conformance", Clock: clock,
		})
		if err != nil {
			t.Fatal(err)
		}
		return runtime
	})
}

func TestPostgresCoordinatorConformance(t *testing.T) {
	db := privacyPostgresDatabase(t)
	privacytest.RunCoordinator(t, func(
		t *testing.T,
		catalog *privacy.Catalog,
		clock func() time.Time,
		router privacy.OwnerRouter,
	) privacy.Coordinator {
		t.Helper()
		coordinator, err := privacy.NewPostgresCoordinator(
			context.Background(), catalog,
			privacy.PostgresOptions{DB: db, InstanceKey: "coordinator-conformance", Clock: clock},
			router,
		)
		if err != nil {
			t.Fatal(err)
		}
		return coordinator
	})
}

func TestPostgresBoundEvidenceRollsBackWithCaller(t *testing.T) {
	db := privacyPostgresDatabase(t)
	now := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	catalog := privacy.MustCompile(privacytest.Definition(now))
	runtime, err := privacy.NewPostgresRuntime(context.Background(), catalog, privacy.PostgresOptions{
		DB: db, InstanceKey: "bound-runtime", Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	subject := privacy.SubjectRef{Owner: "blog", Kind: "address", Value: "rollback@example.test"}
	if _, err := runtime.Bind(tx).Consent(context.Background(), privacy.ConsentCommand{
		IdempotencyKey: "rollback-consent", Subject: subject,
		Notice: privacytest.NewsletterNotice, Purposes: []privacy.PurposeRef{privacytest.NewsletterPurpose},
		OccurredAt: now, Channel: "test",
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	processing, err := runtime.Purpose(privacytest.NewsletterPurpose.Key)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := processing.Decide(context.Background(), privacy.DecisionInput{Subject: privacy.SingleSubject(subject)})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind != privacy.DecisionDeny || decision.Reasons[0] != "consent_missing" {
		t.Fatalf("rolled-back decision = %#v", decision)
	}
}

func privacyPostgresDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("PRIVACY_PG_DSN"))
	if dsn == "" {
		t.Skip("PRIVACY_PG_DSN is not set")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := fmt.Sprintf("privacy_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !strings.HasPrefix(schema, "privacy_test_") {
			t.Fatalf("unsafe schema cleanup target %q", schema)
		}
		_, _ = admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
	})
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	migration, err := privacy.Schema(privacy.CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(migration.UpSQL); err != nil {
		t.Fatal(err)
	}
	return db
}
