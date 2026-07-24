package webhook

import (
	"strings"
	"testing"
)

func TestSchemaContainsDurableTruthAndDownIsReverseSafe(t *testing.T) {
	migration, err := Schema(CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{
		"webhook_events", "webhook_endpoint_revisions", "webhook_subscription_revisions",
		"webhook_deliveries", "webhook_attempts", "webhook_inbound_receipts", "webhook_replay_receipts",
	} {
		if !strings.Contains(migration.UpSQL, table) {
			t.Fatalf("up schema missing %s", table)
		}
		if !strings.Contains(migration.DownSQL, table) {
			t.Fatalf("down schema missing %s", table)
		}
	}
	if len(migration.Digest) != 64 {
		t.Fatalf("digest=%q", migration.Digest)
	}
}
