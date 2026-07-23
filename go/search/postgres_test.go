package search_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/yueli-official/foundation/go/search"
	"github.com/yueli-official/foundation/go/search/searchtest"
)

func TestPostgresConformance(t *testing.T) {
	database := openSearchPostgres(t)
	searchtest.Run(t, func(t *testing.T, catalog *search.Catalog) search.Module {
		t.Helper()
		instance := fmt.Sprintf("search.test.%d", time.Now().UnixNano())
		module, err := search.NewPostgres(context.Background(), catalog, search.PostgresOptions{
			DB: database, InstanceKey: instance, AnalyzerBindings: map[search.AnalyzerKey]string{"test": "pg_catalog.simple"},
		})
		if err != nil {
			t.Fatalf("%v: %v", err, errors.Unwrap(err))
		}
		return module
	})
}

func TestPostgresCallerOwnedTransactionRollback(t *testing.T) {
	database := openSearchPostgres(t)
	catalog := searchtest.Catalog(t)
	module, err := search.NewPostgres(context.Background(), catalog, search.PostgresOptions{
		DB: database, InstanceKey: "search.transaction", AnalyzerBindings: map[search.AnalyzerKey]string{"test": "pg_catalog.simple"},
	})
	if err != nil {
		t.Fatalf("%v: %v", err, errors.Unwrap(err))
	}
	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	projector, err := module.Bind(tx)
	if err != nil {
		t.Fatal(err)
	}
	document := searchtest.Document("rolled-back", 1, "Rollback needle", time.Now().UTC(), nil)
	if _, err := projector.Apply(context.Background(), search.Batch{
		ID: "batch.rollback", Changes: []search.Change{search.Upsert(document)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	page, err := module.Search(context.Background(), search.Query{Text: "needle", Analyzer: "test"})
	if err != nil || page.Total != 0 {
		t.Fatalf("Search after rollback = %#v, %v", page, err)
	}
	if _, err := module.Apply(context.Background(), search.Batch{
		ID: "batch.rollback", Changes: []search.Change{search.Upsert(document)},
	}); err != nil {
		t.Fatalf("rolled-back receipt prevented retry: %v", err)
	}
}

func openSearchPostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SEARCH_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("SEARCH_POSTGRES_DSN is not configured")
	}
	database, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	schema := fmt.Sprintf("search_test_%d", time.Now().UnixNano())
	if _, err := database.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema)); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if _, err := database.Exec("SET search_path TO " + pq.QuoteIdentifier(schema)); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if _, err := database.Exec(search.PostgresMigrationUp); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(search.PostgresMigrationDown)
		_, _ = database.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema))
		_ = database.Close()
	})
	return database
}
