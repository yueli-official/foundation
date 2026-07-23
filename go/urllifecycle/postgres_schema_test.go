package urllifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostgresSchemaIsPrefixableAndImmutable(t *testing.T) {
	migration, err := PostgresSchema(CurrentPostgresSchemaVersion, "docs_urls_")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(migration.UpSQL, "{{prefix}}") ||
		!strings.Contains(migration.UpSQL, "CREATE TABLE docs_urls_instances") {
		t.Fatal("schema prefix was not rendered")
	}
	directory := t.TempDir()
	first, err := WritePostgresMigration(directory, "0008_url_lifecycle_v1", 1, "docs_urls_")
	if err != nil {
		t.Fatal(err)
	}
	second, err := WritePostgresMigration(directory, "0008_url_lifecycle_v1", 1, "docs_urls_")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("idempotent write changed result")
	}
	if err := os.WriteFile(first.UpPath, []byte("drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePostgresMigration(directory, filepath.Base(strings.TrimSuffix(first.UpPath, ".up.sql")), 1, "docs_urls_"); err == nil {
		t.Fatal("migration drift was accepted")
	}
}
