package siteprofile_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/yueli-official/foundation/go/siteprofile"
	"github.com/yueli-official/foundation/go/siteprofile/siteprofiletest"
)

func TestPostgresConformance(t *testing.T) {
	db := openPostgres(t)
	definition := siteprofile.MustCompileDefinition(siteprofile.DefaultDefinition())
	siteprofiletest.Run(t, func(t *testing.T, clock siteprofile.Clock) siteprofile.Module {
		prefix := newPrefix(t)
		installSchema(t, db, prefix)
		store, err := siteprofile.NewPostgresStore(db, prefix)
		if err != nil {
			t.Fatal(err)
		}
		return siteprofile.MustNew(store, definition, clock)
	})
}

func TestPostgresBoundTransactionRollsBack(t *testing.T) {
	db := openPostgres(t)
	prefix := newPrefix(t)
	installSchema(t, db, prefix)
	store, err := siteprofile.NewPostgresStore(db, prefix)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := store.Bind(tx)
	if err != nil {
		t.Fatal(err)
	}
	module := siteprofile.MustNew(
		bound, siteprofile.MustCompileDefinition(siteprofile.DefaultDefinition()), fixedClock{value: time.Now()},
	)
	if _, err := module.Replace(context.Background(), siteprofile.ReplaceCommand{
		Profile: siteprofiletest.ValidProfile(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	outside := siteprofile.MustNew(
		store, siteprofile.MustCompileDefinition(siteprofile.DefaultDefinition()), fixedClock{value: time.Now()},
	)
	if _, err := outside.Get(context.Background()); !errors.Is(err, siteprofile.ErrNotInitialized) {
		t.Fatalf("Get after rollback error = %v, want ErrNotInitialized", err)
	}
}

func openPostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SITE_PROFILE_PG_DSN")
	if dsn == "" {
		t.Skip("SITE_PROFILE_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func installSchema(t *testing.T, db *sql.DB, prefix string) {
	t.Helper()
	migration, err := siteprofile.PostgresSchema(siteprofile.CurrentPostgresSchemaVersion, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), migration.UpSQL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), migration.DownSQL); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})
}

func newPrefix(t *testing.T) string {
	t.Helper()
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	return "spf_" + hex.EncodeToString(raw[:]) + "_"
}

type fixedClock struct{ value time.Time }

func (c fixedClock) Now() time.Time { return c.value }
