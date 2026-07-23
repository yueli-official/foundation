package audit_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/audit"
)

func TestMemoryRetentionHonorsHoldsArchivesAndPreservesIntegrity(t *testing.T) {
	now := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	catalog, contract := retentionCatalog(t)
	module, err := audit.NewMemory(catalog, audit.MemoryOptions{
		Clock: fixedRetentionClock{now}, Source: audit.Source{Service: "docs", Instance: "docs-main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	attempts := make([]audit.Attempt[retentionEvidence], 3)
	for index := range attempts {
		attempts[index] = retentionAttempt(audit.EventID("event-"+string(rune('1'+index))), uint64(index+1))
		if _, err := audit.Record(context.Background(), module, contract, attempts[index]); err != nil {
			t.Fatal(err)
		}
	}
	hold, err := module.PlaceHold(context.Background(), audit.PlaceHoldCommand{
		ID: "case-1", Reason: "legal.request", Actor: audit.Actor{Kind: audit.ActorUser, ID: "admin-1"},
		Selection: audit.HoldSelection{EventIDs: []audit.EventID{"event-2"}},
	})
	if err != nil || hold.ReleasedAt != nil {
		t.Fatalf("PlaceHold() = %#v, %v", hold, err)
	}
	sink := &memoryArchiveSink{}
	receipt, err := module.RunRetention(context.Background(), audit.RetentionCommand{
		ID: "retention-1", AsOf: now.Add(31 * 24 * time.Hour), BatchLimit: 10,
		Actor: audit.Actor{Kind: audit.ActorSystem, ID: "retention-worker"}, Archive: sink,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Deleted != 2 || len(receipt.Ranges) != 2 || receipt.Archive == nil || sink.calls != 1 {
		t.Fatalf("RunRetention() = %#v, sink calls %d", receipt, sink.calls)
	}
	page, err := module.Query(context.Background(), audit.Query{})
	if err != nil || len(page.Events) != 1 || page.Events[0].ID != "event-2" {
		t.Fatalf("Query() after retention = %#v, %v", page, err)
	}
	verified, err := module.Verify(context.Background(), audit.VerifyRequest{})
	if err != nil || !verified.Valid || verified.Count != 1 {
		t.Fatalf("Verify() after sparse purge = %#v, %v", verified, err)
	}
	if _, err := audit.Record(context.Background(), module, contract, attempts[0]); !audit.IsKind(err, audit.ErrorIdempotencyConflict) {
		t.Fatalf("purged event ID replay error = %v", err)
	}
	if _, err := module.ReleaseHold(context.Background(), audit.ReleaseHoldCommand{
		ID: "case-1", Reason: "legal.released", Actor: audit.Actor{Kind: audit.ActorUser, ID: "admin-1"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := module.RunRetention(context.Background(), audit.RetentionCommand{
		ID: "retention-2", AsOf: now.Add(31 * 24 * time.Hour),
		Actor: audit.Actor{Kind: audit.ActorSystem, ID: "retention-worker"}, Archive: sink,
	}); err != nil {
		t.Fatal(err)
	}
	verified, err = module.Verify(context.Background(), audit.VerifyRequest{})
	if err != nil || !verified.Valid || verified.Count != 0 {
		t.Fatalf("Verify() after full purge = %#v, %v", verified, err)
	}
}

func TestMemoryRetentionRequiresMatchingArchiveReceipt(t *testing.T) {
	now := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	catalog, contract := retentionCatalog(t)
	module, err := audit.NewMemory(catalog, audit.MemoryOptions{
		Clock: fixedRetentionClock{now}, Source: audit.Source{Service: "docs", Instance: "docs-main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := audit.Record(context.Background(), module, contract, retentionAttempt("event-1", 1)); err != nil {
		t.Fatal(err)
	}
	_, err = module.RunRetention(context.Background(), audit.RetentionCommand{
		ID: "retention-bad", AsOf: now.Add(31 * 24 * time.Hour),
		Actor:   audit.Actor{Kind: audit.ActorSystem, ID: "retention-worker"},
		Archive: mismatchedArchiveSink{},
	})
	if !audit.IsKind(err, audit.ErrorArchiveRequired) {
		t.Fatalf("RunRetention() error = %v", err)
	}
	page, _ := module.Query(context.Background(), audit.Query{})
	if len(page.Events) != 1 {
		t.Fatalf("failed archive purged events: %#v", page)
	}
}

type retentionEvidence struct{ Revision uint64 }

func retentionCatalog(t *testing.T) (*audit.Catalog, audit.Contract[retentionEvidence]) {
	t.Helper()
	catalog := audit.MustCompile(audit.Definition{
		Version: 1, Consumer: "docs.audit",
		Retention: []audit.RetentionDefinition{{
			Class: "retention.management", MinimumAge: 30 * 24 * time.Hour, ArchiveBefore: true,
		}},
		Actions: []audit.ActionDefinition{{
			Action:   audit.Action{Name: "docs.document.published", Version: 1},
			Category: audit.CategoryAdministration, TargetTypes: []string{"docs.document"},
			Commit: audit.CommitAtomicRequired, Retention: "retention.management",
			Evidence: []audit.FieldDefinition{{Key: "document.revision", Kind: audit.EvidenceCount, Required: true}},
		}},
	})
	contract, err := audit.BindAction(catalog, audit.Action{Name: "docs.document.published", Version: 1}, func(value retentionEvidence) []audit.EvidenceField {
		return []audit.EvidenceField{audit.Count("document.revision", value.Revision)}
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog, contract
}

func retentionAttempt(id audit.EventID, revision uint64) audit.Attempt[retentionEvidence] {
	return audit.Attempt[retentionEvidence]{
		ID: id, Actor: audit.Actor{Kind: audit.ActorUser, ID: "author-1"},
		Target:   audit.Target{Type: "docs.document", ID: "doc-1"},
		Outcome:  audit.Outcome{Kind: audit.OutcomeSucceeded},
		Evidence: retentionEvidence{Revision: revision},
	}
}

type fixedRetentionClock struct{ value time.Time }

func (clock fixedRetentionClock) Now() time.Time { return clock.value }

type memoryArchiveSink struct{ calls int }

func (sink *memoryArchiveSink) Put(
	_ context.Context,
	descriptor audit.ArchiveDescriptor,
	write func(io.Writer) error,
) (audit.ArchiveReceipt, error) {
	sink.calls++
	var output bytes.Buffer
	if err := write(&output); err != nil {
		return audit.ArchiveReceipt{}, err
	}
	sum := sha256.Sum256(output.Bytes())
	return audit.ArchiveReceipt{
		Reference: "archive/" + descriptor.RetentionID,
		Count:     descriptor.ExpectedCount, ContentDigest: audit.Digest(hex.EncodeToString(sum[:])),
	}, nil
}

type mismatchedArchiveSink struct{}

func (mismatchedArchiveSink) Put(
	_ context.Context,
	descriptor audit.ArchiveDescriptor,
	write func(io.Writer) error,
) (audit.ArchiveReceipt, error) {
	if err := write(io.Discard); err != nil {
		return audit.ArchiveReceipt{}, err
	}
	return audit.ArchiveReceipt{
		Reference: "archive/" + descriptor.RetentionID,
		Count:     descriptor.ExpectedCount, ContentDigest: audit.Digest(strings.Repeat("0", 64)),
	}, nil
}
