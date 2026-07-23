package siteprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostgresSchemaAndImmutableWriter(t *testing.T) {
	migration, err := PostgresSchema(CurrentPostgresSchemaVersion, "test_profile_")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(migration.UpSQL, "CREATE TABLE test_profile_state") || migration.Digest == "" {
		t.Fatalf("migration = %#v", migration)
	}
	directory := t.TempDir()
	first, err := WritePostgresMigration(directory, "0001_site_profile", CurrentPostgresSchemaVersion, "test_profile_")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WritePostgresMigration(directory, "0001_site_profile", CurrentPostgresSchemaVersion, "test_profile_"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first.UpPath, []byte("drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePostgresMigration(directory, "0001_site_profile", CurrentPostgresSchemaVersion, "test_profile_"); err == nil {
		t.Fatal("expected migration drift error")
	}
	if filepath.Base(first.DownPath) != "0001_site_profile.down.sql" {
		t.Fatalf("down path = %s", first.DownPath)
	}
}
