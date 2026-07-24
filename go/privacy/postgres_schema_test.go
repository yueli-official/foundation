package privacy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yueli-official/foundation/go/privacy"
)

func TestPrivacySchemaAndImmutableWriter(t *testing.T) {
	migration, err := privacy.Schema(privacy.CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{
		"privacy_instances", "privacy_definition_revisions", "privacy_evidence_events",
		"privacy_retention_items", "privacy_rights_requests", "privacy_owner_tasks",
		"privacy_host_commands",
	} {
		if !strings.Contains(migration.UpSQL, table) {
			t.Fatalf("up migration misses %s", table)
		}
		if !strings.Contains(migration.DownSQL, table) {
			t.Fatalf("down migration misses %s", table)
		}
	}
	directory := t.TempDir()
	first, err := privacy.WriteMigration(directory, "001_privacy", privacy.CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	second, err := privacy.WriteMigration(directory, "001_privacy", privacy.CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("repeat write changed result: %#v != %#v", first, second)
	}
	if err := os.WriteFile(first.UpPath, []byte("drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := privacy.WriteMigration(directory, filepath.Base(strings.TrimSuffix(first.UpPath, ".up.sql")), privacy.CurrentSchemaVersion); err == nil {
		t.Fatal("expected migration drift error")
	}
}
