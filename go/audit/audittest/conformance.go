package audittest

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/audit"
)

type Factory func(t *testing.T, catalog *audit.Catalog, clock audit.Clock) audit.Module

type sampleEvidence struct {
	Revision uint64
	Digest   string
	Fields   []string
}

func Run(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("append returns normalized immutable event", func(t *testing.T) {
		catalog, contract := sampleCatalog(t)
		module := factory(t, catalog, fixedClock{value: instant()})
		event, err := audit.Record(context.Background(), module, contract, attempt("event-1", sampleEvidence{
			Revision: 7, Digest: strings.Repeat("a", 64), Fields: []string{"title", "status"},
		}))
		if err != nil {
			t.Fatal(err)
		}
		if event.Sequence != 1 || event.RecordedAt != instant() || event.OccurredAt != instant().Add(-time.Second) {
			t.Fatalf("event timing/sequence = %#v", event)
		}
		if event.Action.Name != "docs.document.published" || event.RetentionClass != "retention.management" {
			t.Fatalf("event contract = %#v", event)
		}
		if len(event.Evidence) != 3 || event.Evidence[0].Key != "document.changed_fields" {
			t.Fatalf("evidence not canonical = %#v", event.Evidence)
		}
		event.Evidence[0].List[0] = "mutated"
		stored, found, err := module.Get(context.Background(), "event-1")
		if err != nil || !found || stored.Evidence[0].List[0] != "title" {
			t.Fatalf("stored event mutated: found=%v event=%#v err=%v", found, stored, err)
		}
	})

	t.Run("idempotency replay and conflict", func(t *testing.T) {
		catalog, contract := sampleCatalog(t)
		module := factory(t, catalog, fixedClock{value: instant()})
		firstAttempt := attempt("event-replay", sampleEvidence{Revision: 1, Digest: strings.Repeat("b", 64)})
		first, err := audit.Record(context.Background(), module, contract, firstAttempt)
		if err != nil {
			t.Fatal(err)
		}
		replayed, err := audit.Record(context.Background(), module, contract, firstAttempt)
		if err != nil || replayed.Sequence != first.Sequence || replayed.Digest != first.Digest {
			t.Fatalf("replay=%#v err=%v", replayed, err)
		}
		changed := firstAttempt
		changed.Evidence.Revision = 2
		if _, err := audit.Record(context.Background(), module, contract, changed); !audit.IsKind(err, audit.ErrorIdempotencyConflict) {
			t.Fatalf("conflict error = %v", err)
		}
	})

	t.Run("batch is ordered and atomic", func(t *testing.T) {
		catalog, contract := sampleCatalog(t)
		module := factory(t, catalog, fixedClock{value: instant()})
		first, err := audit.Prepare(contract, attempt("batch-1", sampleEvidence{Revision: 1, Digest: strings.Repeat("c", 64)}))
		if err != nil {
			t.Fatal(err)
		}
		second, err := audit.Prepare(contract, attempt("batch-2", sampleEvidence{Revision: 2, Digest: strings.Repeat("d", 64)}))
		if err != nil {
			t.Fatal(err)
		}
		events, err := module.AppendBatch(context.Background(), []audit.Command{first, second})
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 2 || events[0].Sequence != 1 || events[1].Sequence != 2 ||
			events[1].PreviousDigest != events[0].Digest {
			t.Fatalf("batch events = %#v", events)
		}
		if _, err := module.AppendBatch(context.Background(), []audit.Command{first, first}); err == nil {
			t.Fatal("duplicate batch ID accepted")
		}
		page, err := module.Query(context.Background(), audit.Query{})
		if err != nil || len(page.Events) != 2 {
			t.Fatalf("failed batch changed journal: %#v err=%v", page, err)
		}
	})

	t.Run("keyset query is stable and filter-bound", func(t *testing.T) {
		catalog, contract := sampleCatalog(t)
		module := factory(t, catalog, fixedClock{value: instant()})
		for index, actor := range []string{"user-1", "user-2", "user-1"} {
			value := attempt(audit.EventID("query-"+string(rune('a'+index))), sampleEvidence{
				Revision: uint64(index + 1), Digest: strings.Repeat("e", 64),
			})
			value.Actor.ID = actor
			if _, err := audit.Record(context.Background(), module, contract, value); err != nil {
				t.Fatal(err)
			}
		}
		page, err := module.Query(context.Background(), audit.Query{
			Actor: &audit.Actor{Kind: audit.ActorUser, ID: "user-1"}, Limit: 1,
		})
		if err != nil || len(page.Events) != 1 || page.Events[0].Sequence != 3 || page.NextCursor == "" {
			t.Fatalf("first page=%#v err=%v", page, err)
		}
		next, err := module.Query(context.Background(), audit.Query{
			Actor: &audit.Actor{Kind: audit.ActorUser, ID: "user-1"}, Limit: 1, Before: page.NextCursor,
		})
		if err != nil || len(next.Events) != 1 || next.Events[0].Sequence != 1 {
			t.Fatalf("next page=%#v err=%v", next, err)
		}
		if _, err := module.Query(context.Background(), audit.Query{
			Actor: &audit.Actor{Kind: audit.ActorUser, ID: "user-2"}, Limit: 1, Before: page.NextCursor,
		}); !audit.IsKind(err, audit.ErrorInvalidCursor) {
			t.Fatalf("cursor reused across filter: %v", err)
		}
	})

	t.Run("export is streaming and integrity verifies", func(t *testing.T) {
		catalog, contract := sampleCatalog(t)
		module := factory(t, catalog, fixedClock{value: instant()})
		if _, err := audit.Record(context.Background(), module, contract, attempt(
			"export-1", sampleEvidence{Revision: 1, Digest: strings.Repeat("f", 64)},
		)); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		manifest, err := module.Export(context.Background(), audit.ExportRequest{}, &output)
		if err != nil {
			t.Fatal(err)
		}
		if manifest.Count != 1 || manifest.ContentDigest == "" ||
			!strings.Contains(output.String(), `"kind":"audit.event"`) {
			t.Fatalf("manifest=%#v output=%s", manifest, output.String())
		}
		verified, err := module.Verify(context.Background(), audit.VerifyRequest{})
		if err != nil || !verified.Valid || verified.Count != 1 || verified.HeadDigest == "" {
			t.Fatalf("verify=%#v err=%v", verified, err)
		}
	})
}

func sampleCatalog(t *testing.T) (*audit.Catalog, audit.Contract[sampleEvidence]) {
	t.Helper()
	catalog, err := audit.Compile(audit.Definition{
		Version: 1, Consumer: "docs.main", MaxBatch: 20, MaxEvidence: 10,
		Retention: []audit.RetentionDefinition{
			{Class: "retention.management", MinimumAge: 90 * 24 * time.Hour, ArchiveBefore: true},
		},
		Actions: []audit.ActionDefinition{
			{
				Action:   audit.Action{Name: "docs.document.published", Version: 1},
				Category: audit.CategoryAdministration, TargetTypes: []string{"docs.document"},
				Commit: audit.CommitAtomicRequired, Retention: "retention.management",
				Evidence: []audit.FieldDefinition{
					{Key: "document.revision", Kind: audit.EvidenceCount, Required: true},
					{Key: "document.digest", Kind: audit.EvidenceDigest, Required: true},
					{Key: "document.changed_fields", Kind: audit.EvidenceCodeList, MaxItems: 10, MaxLength: 64},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := audit.BindAction(catalog, audit.Action{Name: "docs.document.published", Version: 1}, func(value sampleEvidence) []audit.EvidenceField {
		fields := []audit.EvidenceField{
			audit.Count("document.revision", value.Revision),
			audit.EvidenceDigestValue("document.digest", value.Digest),
		}
		if len(value.Fields) > 0 {
			fields = append(fields, audit.Codes("document.changed_fields", value.Fields...))
		}
		return fields
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog, contract
}

func attempt(id audit.EventID, evidence sampleEvidence) audit.Attempt[sampleEvidence] {
	return audit.Attempt[sampleEvidence]{
		ID: id, Actor: audit.Actor{Kind: audit.ActorUser, ID: "user-1"},
		Target:  audit.Target{Type: "docs.document", ID: "doc-1"},
		Outcome: audit.Outcome{Kind: audit.OutcomeSucceeded},
		Correlation: audit.Correlation{
			RequestID: "request-1", TraceID: strings.Repeat("a", 32), SpanID: strings.Repeat("b", 16),
			CommandID: "command-1",
		},
		OccurredAt: instant().Add(-time.Second), Evidence: evidence,
	}
}

type fixedClock struct{ value time.Time }

func (clock fixedClock) Now() time.Time { return clock.value }

func instant() time.Time {
	return time.Date(2026, 7, 24, 1, 0, 0, 0, time.UTC)
}
