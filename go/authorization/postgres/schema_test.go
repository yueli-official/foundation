package postgres_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yueli-official/foundation/go/authorization/postgres"
)

func TestSchemaV1ContainsDomainTruthAndRebuildableProjection(t *testing.T) {
	migration, err := postgres.Schema(1)
	if err != nil {
		t.Fatalf("Schema(1) error = %v", err)
	}
	if migration.Version != 1 || migration.Digest == "" {
		t.Fatalf("Schema(1) = %#v, want version and digest", migration)
	}
	for _, table := range []string{
		"authorization_instances",
		"authorization_scopes",
		"authorization_policy_revisions",
		"authorization_policy_scopes",
		"authorization_role_definitions",
		"authorization_role_policies",
		"authorization_policy_bindings",
		"authorization_automatic_rules",
		"authorization_groups",
		"authorization_group_members",
		"authorization_grants",
		"authorization_applications",
		"authorization_invitations",
		"authorization_inbox_events",
		"authorization_audit_events",
		"authorization_decision_events",
		"authorization_projection_rules",
	} {
		if !strings.Contains(migration.UpSQL, "CREATE TABLE "+table) {
			t.Errorf("Schema(1).UpSQL does not create %s", table)
		}
	}
	if !strings.Contains(migration.DownSQL, "DROP TABLE authorization_instances") {
		t.Fatal("Schema(1).DownSQL does not remove the instance root")
	}
	if _, err := postgres.Schema(2); err == nil {
		t.Fatal("Schema(2) succeeded, want unsupported-version error")
	}
}

func TestWriteMigrationIsDeterministicAndDetectsDrift(t *testing.T) {
	directory := t.TempDir()
	written, err := postgres.WriteMigration(directory, "0007_authorization_v1", 1)
	if err != nil {
		t.Fatalf("WriteMigration() error = %v", err)
	}
	up, err := os.ReadFile(written.UpPath)
	if err != nil {
		t.Fatalf("ReadFile(up) error = %v", err)
	}
	if !strings.Contains(string(up), "-- authorization-schema-version: 1") ||
		!strings.Contains(string(up), "-- authorization-schema-digest: ") {
		t.Fatalf("generated header = %q, want version and digest", up[:min(len(up), 160)])
	}
	second, err := postgres.WriteMigration(directory, "0007_authorization_v1", 1)
	if err != nil {
		t.Fatalf("WriteMigration() repeat error = %v", err)
	}
	if second != written {
		t.Fatalf("WriteMigration() repeat = %#v, want %#v", second, written)
	}
	if err := os.WriteFile(written.UpPath, append(up, []byte("\n-- local drift\n")...), 0o644); err != nil {
		t.Fatalf("WriteFile(drift) error = %v", err)
	}
	if _, err := postgres.WriteMigration(directory, "0007_authorization_v1", 1); err == nil {
		t.Fatal("WriteMigration() over drift succeeded, want conflict")
	}
	if filepath.Base(written.DownPath) != "0007_authorization_v1.down.sql" {
		t.Fatalf("DownPath = %q", written.DownPath)
	}
}
