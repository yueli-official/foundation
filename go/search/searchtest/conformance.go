package searchtest

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/search"
)

type Factory func(*testing.T, *search.Catalog) search.Module

func Run(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("projection-query-cursor", func(t *testing.T) {
		module := factory(t, Catalog(t))
		now := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
		changes := make([]search.Change, 4)
		for index := range changes {
			changes[index] = search.Upsert(Document(
				fmt.Sprintf("doc-%d", index+1), uint64(index+1),
				fmt.Sprintf("Search title %d", index+1), now.Add(time.Duration(index)*time.Minute),
				map[search.FilterName]search.Value{"collection": search.Keyword("guide")},
			))
		}
		if result, err := module.Apply(context.Background(), search.Batch{ID: "batch.initial", Changes: changes}); err != nil || result.Applied != 4 {
			t.Fatalf("Apply() = %#v, %v: %v", result, err, errors.Unwrap(err))
		}
		query := search.Query{
			Text: "search", Analyzer: "test", Filters: []search.Filter{search.Equal("collection", "guide")},
			Facets: []search.FacetRequest{{Name: "collection", Limit: 5}}, Page: search.PageRequest{Size: 2},
			Highlight: search.HighlightRequest{Fields: []search.TextField{search.TextTitle}},
		}
		first, err := module.Search(context.Background(), query)
		if err != nil || len(first.Hits) != 2 || first.Total != 4 || first.NextCursor == "" ||
			len(first.Facets) != 1 || first.Facets[0].Buckets[0].Count != 4 {
			t.Fatalf("Search(first) = %#v, %v", first, err)
		}
		query.Page.Cursor = first.NextCursor
		second, err := module.Search(context.Background(), query)
		if err != nil || len(second.Hits) != 2 || second.Hits[0].Key == first.Hits[0].Key {
			t.Fatalf("Search(second) = %#v, %v", second, err)
		}
		query.Text = "different"
		if _, err := module.Search(context.Background(), query); !search.IsKind(err, search.ErrorInvalidCursor) {
			t.Fatalf("Search(cursor mismatch) error = %v", err)
		}
	})

	t.Run("revision-tombstone-atomicity", func(t *testing.T) {
		module := factory(t, Catalog(t))
		now := time.Now().UTC()
		initial := Document("doc-1", 2, "Current search", now, nil)
		if _, err := module.Apply(context.Background(), search.Batch{ID: "batch.current", Changes: []search.Change{search.Upsert(initial)}}); err != nil {
			t.Fatal(err)
		}
		stale := Document("doc-1", 1, "Stale search", now, nil)
		result, err := module.Apply(context.Background(), search.Batch{ID: "batch.stale", Changes: []search.Change{search.Upsert(stale)}})
		if err != nil || result.Stale != 1 {
			t.Fatalf("stale Apply() = %#v, %v", result, err)
		}
		conflict := Document("doc-1", 2, "Conflicting search", now, nil)
		batch := search.Batch{ID: "batch.conflict", Changes: []search.Change{
			search.Upsert(Document("doc-2", 1, "Must roll back search", now, nil)),
			search.Upsert(conflict),
		}}
		if _, err := module.Apply(context.Background(), batch); !search.IsKind(err, search.ErrorRevisionConflict) {
			t.Fatalf("conflict error = %v", err)
		}
		page, _ := module.Search(context.Background(), search.Query{Text: "roll back", Analyzer: "test"})
		if page.Total != 0 {
			t.Fatalf("atomic batch leaked document: %#v", page)
		}
		if _, err := module.Apply(context.Background(), search.Batch{ID: "batch.remove", Changes: []search.Change{
			search.Remove(initial.Key, 3),
		}}); err != nil {
			t.Fatalf("%v: %v", err, errors.Unwrap(err))
		}
		page, _ = module.Search(context.Background(), search.Query{Text: "current", Analyzer: "test"})
		if page.Total != 0 {
			t.Fatalf("removed document still matches: %#v", page)
		}
	})

	t.Run("rebuild-dual-write", func(t *testing.T) {
		module := factory(t, Catalog(t))
		now := time.Now().UTC()
		state, err := module.Start(context.Background(), search.StartRebuild{RequestID: "rebuild.one"})
		if err != nil {
			t.Fatalf("%v: %v", err, errors.Unwrap(err))
		}
		old := Document("doc-1", 1, "Old text", now, nil)
		if _, err := module.Stage(context.Background(), search.RebuildBatch{
			Generation: state.Generation, Batch: search.Batch{ID: "rebuild.batch.one", Changes: []search.Change{search.Upsert(old)}},
			Checkpoint: "doc-1",
		}); err != nil {
			t.Fatal(err)
		}
		current := Document("doc-1", 2, "Fresh searchable text", now.Add(time.Minute), nil)
		if _, err := module.Apply(context.Background(), search.Batch{ID: "batch.fresh", Changes: []search.Change{search.Upsert(current)}}); err != nil {
			t.Fatal(err)
		}
		if _, err := module.Finish(context.Background(), search.FinishRebuild{
			Generation: state.Generation, FinalCheckpoint: "doc-1", ExpectedDocuments: 1,
		}); err != nil {
			t.Fatal(err)
		}
		page, err := module.Search(context.Background(), search.Query{Text: "fresh", Analyzer: "test"})
		if err != nil || page.Total != 1 || page.Hits[0].Revision != 2 {
			t.Fatalf("Search after rebuild = %#v, %v", page, err)
		}
	})
}

func Catalog(t *testing.T) *search.Catalog {
	t.Helper()
	catalog, err := search.Compile(search.Definition{
		Consumer: "search.test", Version: 1,
		Analyzers: []search.AnalyzerDefinition{{Key: "test", QueryMode: search.QueryPlain, Required: []search.Capability{search.CapabilityFullText}}},
		Filters:   []search.FilterDefinition{{Name: "collection", Facetable: true}},
		Limits:    search.Limits{DefaultPageSize: 2, MaxPageSize: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func Document(id string, revision uint64, title string, at time.Time, fields map[search.FilterName]search.Value) search.SourceDocument {
	return search.SourceDocument{
		Key: search.DocumentKey{Kind: "document", ID: search.DocumentID(id)}, Revision: search.ProjectionRevision(revision),
		Analyzer: "test", Title: title, Summary: "summary", Body: "body", Filters: fields, SortAt: at,
		Visibility: search.VisibilityReference{ResourceType: "document", ResourceID: id},
	}
}
