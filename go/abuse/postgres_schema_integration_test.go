package abuse_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"testing"

	"github.com/lib/pq"
	"github.com/yueli-official/foundation/go/abuse"
)

func TestPostgresSchemaMigrationUpDown(t *testing.T) {
	dsn := os.Getenv("ABUSE_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("ABUSE_POSTGRES_DSN is not set")
	}
	database, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	connection, err := database.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	schema := "abuse_migration_test_" + hex.EncodeToString(random)
	quoted := pq.QuoteIdentifier(schema)
	if _, err := connection.ExecContext(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = connection.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+quoted+" CASCADE")
	}()
	if _, err := connection.ExecContext(ctx, "SET search_path TO "+quoted); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(ctx, abuse.PostgresSchemaUp()); err != nil {
		t.Fatalf("apply up migration: %v", err)
	}
	for _, table := range []string{
		"abuse_instances",
		"abuse_policy_definitions",
		"abuse_meter_states",
		"abuse_attempt_receipts",
	} {
		var exists bool
		if err := connection.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = $1 AND table_name = $2
)`, schema, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("up migration did not create %s", table)
		}
	}
	if _, err := connection.ExecContext(ctx, abuse.PostgresSchemaDown()); err != nil {
		t.Fatalf("apply down migration: %v", err)
	}
	var remaining int
	if err := connection.QueryRowContext(ctx, `
SELECT count(*) FROM information_schema.tables
WHERE table_schema = $1 AND table_name LIKE 'abuse_%'`, schema).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("down migration left %d abuse tables", remaining)
	}
}
