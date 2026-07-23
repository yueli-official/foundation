package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/yueli-official/foundation/go/urllifecycle"
	"github.com/yueli-official/foundation/go/urllifecycle/postgres"
	"github.com/yueli-official/foundation/go/urllifecycle/urllifecycletest"
)

func TestAdapterConformance(t *testing.T) {
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
	prefix := "url_lifecycle_conf_" + strconv.FormatInt(time.Now().UnixNano(), 36) + "_"
	migration, err := postgres.Schema(postgres.CurrentSchemaVersion, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, migration.UpSQL); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := db.ExecContext(cleanup, migration.DownSQL); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}()

	urllifecycletest.Run(t, func(t *testing.T, catalog *urllifecycle.Catalog) urllifecycle.Module {
		t.Helper()
		adapter, err := postgres.New(ctx, catalog, postgres.Options{
			DB: db, InstanceKey: "conformance", Prefix: prefix,
		})
		if err != nil {
			t.Fatal(err)
		}
		return adapter
	})
}
