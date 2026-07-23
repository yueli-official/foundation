package postgres_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yueli-official/foundation/go/work/postgres"
)

func TestSchemaV1ContainsWorkTruth(t *testing.T) {
	migration, err := postgres.Schema(postgres.CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	if migration.Version != 1 || migration.Digest == "" {
		t.Fatalf("unexpected migration metadata: %+v", migration)
	}
	for _, table := range []string{
		"work_instances", "work_jobs", "work_attempts", "work_schedules",
	} {
		if !strings.Contains(migration.UpSQL, "CREATE TABLE "+table) {
			t.Errorf("schema does not create %s", table)
		}
	}
	for _, index := range []string{
		"work_jobs_idempotency_unique", "work_jobs_due_claim_idx",
		"work_jobs_expired_lease_idx", "work_jobs_retention_idx",
	} {
		if !strings.Contains(migration.UpSQL, index) {
			t.Errorf("schema does not create %s", index)
		}
	}
	if !strings.Contains(migration.DownSQL, "DROP TABLE IF EXISTS work_instances") {
		t.Fatal("down schema does not remove work_instances")
	}
	if _, err := postgres.Schema(2); err == nil {
		t.Fatal("unsupported schema version succeeded")
	}
}

func TestWriteMigrationIsImmutable(t *testing.T) {
	directory := t.TempDir()
	written, err := postgres.WriteMigration(directory, "0015_work_v1", 1)
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile(written.UpPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(up), "-- work-schema-version: 1") ||
		!strings.Contains(string(up), "-- work-schema-digest: ") {
		t.Fatalf("missing generated metadata: %q", up[:min(len(up), 180)])
	}
	repeated, err := postgres.WriteMigration(directory, "0015_work_v1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if repeated != written {
		t.Fatalf("repeat changed result: %+v != %+v", repeated, written)
	}
	if err := os.WriteFile(written.UpPath, append(up, []byte("\n-- drift\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.WriteMigration(directory, "0015_work_v1", 1); err == nil {
		t.Fatal("migration drift was overwritten")
	}
	if filepath.Base(written.DownPath) != "0015_work_v1.down.sql" {
		t.Fatalf("unexpected down path %q", written.DownPath)
	}
}
