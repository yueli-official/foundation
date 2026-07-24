package classification_test

import (
	"reflect"
	"testing"

	"github.com/yueli-official/foundation/go/classification"
)

func TestDiscoverBuildsCanonicalPlanWithDescendants(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  21,
		Categories: []classification.Category{
			{ID: "visual", Slug: "visual", Name: "视觉作品", Status: classification.StatusActive},
			{ID: "wallpaper", ParentID: "visual", Slug: "wallpaper", Name: "壁纸", Status: classification.StatusActive},
			{ID: "desktop", ParentID: "wallpaper", Slug: "desktop", Name: "桌面壁纸", Status: classification.StatusActive},
		},
		Facets: []classification.Facet{
			{ID: "scene", Slug: "scene", Name: "场景", Status: classification.StatusActive},
		},
		FacetValues: []classification.FacetValue{
			{ID: "outdoor", FacetID: "scene", Slug: "outdoor", Name: "户外", Status: classification.StatusActive},
			{ID: "landscape", FacetID: "scene", ParentID: "outdoor", Slug: "landscape", Name: "风景", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1},
		},
	})
	if compiled.Outcome != classification.OutcomeAccepted {
		t.Fatalf("compile diagnostics = %#v", compiled.Diagnostics)
	}

	preparation := compiled.Catalog.Discover(classification.DiscoverRequest{
		PolicyKey: "gallery.image.public",
		Categories: []classification.Reference{
			{Kind: classification.ReferenceBySlug, Value: "wallpaper"},
		},
		Facets: []classification.FacetFilter{
			{
				Facet: classification.Reference{Kind: classification.ReferenceBySlug, Value: "scene"},
				Values: []classification.Reference{
					{Kind: classification.ReferenceBySlug, Value: "outdoor"},
					{Kind: classification.ReferenceBySlug, Value: "landscape"},
				},
			},
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.DiscoverFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeAccepted {
		t.Fatalf("outcome = %q, diagnostics = %#v", result.Outcome, result.Diagnostics)
	}
	if !reflect.DeepEqual(result.FilterPlan.Groups, []classification.FilterGroup{
		{
			Kind:     classification.FilterGroupCategory,
			ValueIDs: []string{"desktop", "wallpaper"},
		},
		{
			Kind:     classification.FilterGroupFacet,
			OwnerID:  "scene",
			ValueIDs: []string{"landscape", "outdoor"},
		},
	}) {
		t.Fatalf("groups = %#v", result.FilterPlan.Groups)
	}
}

func TestDiscoverDoesNotSilentlyBroadenUnknownCategoryGroup(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  22,
		Categories: []classification.Category{
			{ID: "wallpaper", Slug: "wallpaper", Name: "壁纸", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1},
		},
	})

	preparation := compiled.Catalog.Discover(classification.DiscoverRequest{
		PolicyKey: "gallery.image.public",
		Categories: []classification.Reference{
			{Kind: classification.ReferenceBySlug, Value: "missing"},
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.DiscoverFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeNonExecutable {
		t.Fatalf("outcome = %q, diagnostics = %#v", result.Outcome, result.Diagnostics)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticCategoryUnknown {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if len(result.FilterPlan.Groups) != 0 {
		t.Fatalf("filter plan = %#v", result.FilterPlan)
	}
}

func TestDiscoverKeepsValidFacetValueWhileReportingUnknownSibling(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  23,
		Facets: []classification.Facet{
			{ID: "scene", Slug: "scene", Name: "场景", Status: classification.StatusActive},
		},
		FacetValues: []classification.FacetValue{
			{ID: "landscape", FacetID: "scene", Slug: "landscape", Name: "风景", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1},
		},
	})

	preparation := compiled.Catalog.Discover(classification.DiscoverRequest{
		PolicyKey: "gallery.image.public",
		Facets: []classification.FacetFilter{
			{
				Facet: classification.Reference{Kind: classification.ReferenceBySlug, Value: "scene"},
				Values: []classification.Reference{
					{Kind: classification.ReferenceBySlug, Value: "landscape"},
					{Kind: classification.ReferenceBySlug, Value: "missing"},
				},
			},
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.DiscoverFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeAccepted {
		t.Fatalf("outcome = %q, diagnostics = %#v", result.Outcome, result.Diagnostics)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticFacetValueUnknown {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if !reflect.DeepEqual(result.FilterPlan.Groups, []classification.FilterGroup{
		{Kind: classification.FilterGroupFacet, OwnerID: "scene", ValueIDs: []string{"landscape"}},
	}) {
		t.Fatalf("filter plan = %#v", result.FilterPlan)
	}
}

func TestDiscoverProjectsEffectiveActiveCategoryAndFacetTrees(t *testing.T) {
	positionZero := 0
	positionOne := 1
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  24,
		Categories: []classification.Category{
			{ID: "visual", Slug: "visual", Name: "视觉作品", Status: classification.StatusActive, EditorialPosition: &positionZero},
			{ID: "wallpaper", ParentID: "visual", Slug: "wallpaper", Name: "壁纸", Status: classification.StatusActive, EditorialPosition: &positionOne},
			{ID: "hidden", Slug: "hidden", Name: "隐藏", Status: classification.StatusInactive},
		},
		Facets: []classification.Facet{
			{ID: "scene", Slug: "scene", Name: "场景", Status: classification.StatusActive},
		},
		FacetValues: []classification.FacetValue{
			{ID: "outdoor", FacetID: "scene", Slug: "outdoor", Name: "户外", Status: classification.StatusActive},
			{ID: "landscape", FacetID: "scene", ParentID: "outdoor", Slug: "landscape", Name: "风景", Status: classification.StatusActive},
			{ID: "hidden-value", FacetID: "scene", ParentID: "outdoor", Slug: "hidden", Name: "隐藏", Status: classification.StatusInactive},
		},
		Policies: []classification.PolicyProfile{
			{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1},
		},
	})

	preparation := compiled.Catalog.Discover(classification.DiscoverRequest{PolicyKey: "gallery.image.public", CandidateProjection: classification.CandidateProjectionAvailable})
	factRequest := preparation.FactRequest()
	if !reflect.DeepEqual(factRequest.CountGroups[1].Candidates[0].MatchingIDs, []string{"landscape", "outdoor"}) {
		t.Fatalf("inactive descendants leaked into count bucket: %#v", factRequest.CountGroups[1].Candidates[0])
	}
	result := preparation.Complete(classification.DiscoverFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
		FreshnessToken:  "counts:24",
		CountGroups: []classification.CandidateCountGroup{
			{
				Kind:   classification.FilterGroupCategory,
				Counts: []classification.CandidateCount{{ValueID: "visual", Count: 2}, {ValueID: "wallpaper", Count: 1}},
			},
			{
				Kind: classification.FilterGroupFacet, OwnerID: "scene",
				Counts: []classification.CandidateCount{{ValueID: "outdoor", Count: 2}, {ValueID: "landscape", Count: 1}},
			},
		},
	})

	if result.Outcome != classification.OutcomeAccepted {
		t.Fatalf("outcome = %q, diagnostics = %#v", result.Outcome, result.Diagnostics)
	}
	if !reflect.DeepEqual(result.Candidates.Categories, []classification.CandidateNode{
		{ID: "visual", Slug: "visual", Name: "视觉作品", Count: 2},
		{ID: "wallpaper", ParentID: "visual", Slug: "wallpaper", Name: "壁纸", Count: 1},
	}) {
		t.Fatalf("category candidates = %#v", result.Candidates.Categories)
	}
	if !reflect.DeepEqual(result.Candidates.Facets, []classification.CandidateFacet{
		{
			ID: "scene", Slug: "scene", Name: "场景",
			Values: []classification.CandidateNode{
				{ID: "outdoor", Slug: "outdoor", Name: "户外", Count: 2},
				{ID: "landscape", ParentID: "outdoor", Slug: "landscape", Name: "风景", Count: 1},
			},
		},
	}) {
		t.Fatalf("facet candidates = %#v", result.Candidates.Facets)
	}
}

func TestDiscoverCandidateFactsUseSelfExcludingPlansAndRetainSelectedZeroPath(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery", Revision: 26,
		Categories: []classification.Category{
			{ID: "visual", Slug: "visual", Name: "视觉", Status: classification.StatusActive},
			{ID: "wallpaper", ParentID: "visual", Slug: "wallpaper", Name: "壁纸", Status: classification.StatusActive},
			{ID: "photo", Slug: "photo", Name: "摄影", Status: classification.StatusActive},
		},
		Facets:      []classification.Facet{{ID: "scene", Slug: "scene", Name: "场景", Status: classification.StatusActive}},
		FacetValues: []classification.FacetValue{{ID: "landscape", FacetID: "scene", Slug: "landscape", Name: "风景", Status: classification.StatusActive}},
		Policies:    []classification.PolicyProfile{{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1}},
	})
	preparation := compiled.Catalog.Discover(classification.DiscoverRequest{
		PolicyKey: "gallery.image.public", CandidateProjection: classification.CandidateProjectionAvailable,
		Categories: []classification.Reference{{Kind: classification.ReferenceBySlug, Value: "wallpaper"}},
		Facets: []classification.FacetFilter{{
			Facet:  classification.Reference{Kind: classification.ReferenceBySlug, Value: "scene"},
			Values: []classification.Reference{{Kind: classification.ReferenceBySlug, Value: "landscape"}},
		}},
	})
	factRequest := preparation.FactRequest()
	if len(factRequest.CountGroups) != 2 {
		t.Fatalf("count groups = %#v", factRequest.CountGroups)
	}
	if len(factRequest.CountGroups[0].OtherFilters.Groups) != 1 || factRequest.CountGroups[0].OtherFilters.Groups[0].Kind != classification.FilterGroupFacet {
		t.Fatalf("category count plan = %#v", factRequest.CountGroups[0].OtherFilters)
	}
	if len(factRequest.CountGroups[1].OtherFilters.Groups) != 1 || factRequest.CountGroups[1].OtherFilters.Groups[0].Kind != classification.FilterGroupCategory {
		t.Fatalf("facet count plan = %#v", factRequest.CountGroups[1].OtherFilters)
	}
	result := preparation.Complete(classification.DiscoverFacts{
		CatalogRevision: factRequest.CatalogRevision, RequestToken: factRequest.RequestToken, FreshnessToken: "counts:26",
		CountGroups: []classification.CandidateCountGroup{
			{Kind: classification.FilterGroupCategory, Counts: []classification.CandidateCount{{ValueID: "visual"}, {ValueID: "wallpaper"}, {ValueID: "photo"}}},
			{Kind: classification.FilterGroupFacet, OwnerID: "scene", Counts: []classification.CandidateCount{{ValueID: "landscape"}}},
		},
	})
	if result.Outcome != classification.OutcomeAccepted {
		t.Fatalf("outcome = %q, diagnostics = %#v", result.Outcome, result.Diagnostics)
	}
	if !reflect.DeepEqual(result.Candidates.Categories, []classification.CandidateNode{
		{ID: "visual", Slug: "visual", Name: "视觉"},
		{ID: "wallpaper", ParentID: "visual", Slug: "wallpaper", Name: "壁纸", Selected: true},
	}) {
		t.Fatalf("categories = %#v", result.Candidates.Categories)
	}
	if !reflect.DeepEqual(result.Candidates.Facets[0].Values, []classification.CandidateNode{
		{ID: "landscape", Slug: "landscape", Name: "风景", Selected: true},
	}) {
		t.Fatalf("facet values = %#v", result.Candidates.Facets)
	}
}

func TestDiscoverRejectsUnknownPolicyKey(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  25,
		Policies: []classification.PolicyProfile{
			{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1},
		},
	})
	preparation := compiled.Catalog.Discover(classification.DiscoverRequest{PolicyKey: "missing"})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.DiscoverFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticPolicyUnknown {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestDiscoverActiveCandidateProjectionKeepsAssignableZeroCountNodes(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery", Revision: 27,
		Categories: []classification.Category{
			{ID: "wallpaper", Slug: "wallpaper", Name: "壁纸", Status: classification.StatusActive},
			{ID: "inactive", Slug: "inactive", Name: "停用", Status: classification.StatusInactive},
		},
		Facets: []classification.Facet{
			{ID: "scene", Slug: "scene", Name: "场景", Status: classification.StatusActive},
		},
		FacetValues: []classification.FacetValue{
			{ID: "landscape", FacetID: "scene", Slug: "landscape", Name: "风景", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1}},
	})
	preparation := compiled.Catalog.Discover(classification.DiscoverRequest{
		PolicyKey: "gallery.image.public", CandidateProjection: classification.CandidateProjectionActive,
	})
	factRequest := preparation.FactRequest()
	if len(factRequest.CountGroups) != 0 {
		t.Fatalf("active projection requested contextual counts: %#v", factRequest.CountGroups)
	}
	result := preparation.Complete(classification.DiscoverFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})
	if result.Outcome != classification.OutcomeAccepted {
		t.Fatalf("outcome = %q, diagnostics = %#v", result.Outcome, result.Diagnostics)
	}
	if !reflect.DeepEqual(result.Candidates.Categories, []classification.CandidateNode{
		{ID: "wallpaper", Slug: "wallpaper", Name: "壁纸"},
	}) {
		t.Fatalf("categories = %#v", result.Candidates.Categories)
	}
	if !reflect.DeepEqual(result.Candidates.Facets, []classification.CandidateFacet{{
		ID: "scene", Slug: "scene", Name: "场景",
		Values: []classification.CandidateNode{{ID: "landscape", Slug: "landscape", Name: "风景"}},
	}}) {
		t.Fatalf("facets = %#v", result.Candidates.Facets)
	}
}

func TestDiscoverRejectsUnknownCandidateProjection(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery", Revision: 28,
		Policies: []classification.PolicyProfile{{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1}},
	})
	preparation := compiled.Catalog.Discover(classification.DiscoverRequest{
		PolicyKey: "gallery.image.public", CandidateProjection: classification.CandidateProjectionMode("invented"),
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.DiscoverFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})
	if result.Outcome != classification.OutcomeRejected || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticCandidateProjectionInvalid {
		t.Fatalf("result = %#v", result)
	}
}
