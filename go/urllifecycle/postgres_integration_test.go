package urllifecycle

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestPostgresAdapterConformance(t *testing.T) {
	dsn := os.Getenv("URL_LIFECYCLE_PG_DSN")
	if dsn == "" {
		t.Skip("URL_LIFECYCLE_PG_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	prefix := "url_lifecycle_test_" + strconv.FormatInt(time.Now().UnixNano(), 36) + "_"
	migration, err := PostgresSchema(CurrentPostgresSchemaVersion, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, migration.UpSQL); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := db.ExecContext(cleanupCtx, migration.DownSQL); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}()

	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	catalog := testCatalog(t)
	adapter, err := NewPostgres(ctx, catalog, PostgresOptions{
		DB: db, InstanceKey: "integration", Prefix: prefix,
		Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !adapter.InstanceWasCreated() {
		t.Fatal("first adapter did not create the instance")
	}
	key := route("page", "post", "")
	created, err := adapter.Apply(ctx, Claim(
		MutationMeta{CommandID: "create", Reason: "publish"},
		ClaimSpec{Route: key, Active: ActiveRoute{Canonical: local("/old")}},
	), ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := adapter.Bind(tx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = bound.Apply(ctx, Rename(
		MutationMeta{CommandID: "rolled-back", Reason: "rollback test"},
		key, created.RouteRevisions[0].Revision, ActiveRoute{Canonical: local("/old")},
		local("/not-committed"), DefaultPermanentRedirect(),
	), ApplyOptions{})
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	stillCanonical, err := adapter.Resolve(ctx, Lookup{EscapedPath: "/old"})
	if err != nil {
		t.Fatal(err)
	}
	if stillCanonical.Kind != ResolutionCanonical {
		t.Fatalf("caller rollback leaked URL state: %#v", stillCanonical)
	}

	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	bound, err = adapter.Bind(tx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = bound.Apply(ctx, Rename(
		MutationMeta{CommandID: "committed", Reason: "commit test"},
		key, created.RouteRevisions[0].Revision, ActiveRoute{Canonical: local("/old")},
		local("/new"), DefaultPermanentRedirect(),
	), ApplyOptions{})
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	redirected, err := adapter.Resolve(ctx, Lookup{EscapedPath: "/old"})
	if err != nil {
		t.Fatal(err)
	}
	if redirected.Kind != ResolutionRedirect || redirected.Location != "/new" {
		t.Fatalf("committed rename did not persist: %#v", redirected)
	}

	reopened, err := NewPostgres(ctx, catalog, PostgresOptions{
		DB: db, InstanceKey: "integration", Prefix: prefix,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.InstanceWasCreated() {
		t.Fatal("reopened adapter reported a new instance")
	}
	reloaded, err := reopened.Resolve(ctx, Lookup{EscapedPath: "/new"})
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Kind != ResolutionCanonical {
		t.Fatalf("reopened adapter lost canonical: %#v", reloaded)
	}
}
