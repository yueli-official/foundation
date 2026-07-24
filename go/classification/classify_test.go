package classification_test

import (
	"reflect"
	"testing"

	"github.com/yueli-official/foundation/go/classification"
)

func TestClassifyPreservesExplicitAncestorAndDescendantAssignments(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  11,
		Categories: []classification.Category{
			{ID: "visual", Slug: "visual", Name: "视觉作品", Status: classification.StatusActive},
			{ID: "wallpaper", ParentID: "visual", Slug: "wallpaper", Name: "壁纸", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{
				Key:            "gallery.image.public",
				SchemaVersion:  1,
				PolicyRevision: 2,
				Category: classification.CategoryPolicy{
					MinAssignments: 1,
					MaxAssignments: 3,
					RequirePrimary: true,
				},
				Tags: classification.TagAdmissionPolicy{Unknown: classification.UnknownTagPropose},
			},
		},
	})
	if compiled.Outcome != classification.OutcomeAccepted {
		t.Fatalf("compile diagnostics = %#v", compiled.Diagnostics)
	}

	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{
		PolicyKey:         "gallery.image.public",
		CategoryIDs:       []string{"wallpaper", "visual", "wallpaper"},
		PrimaryCategoryID: "wallpaper",
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.ClassifyFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeAccepted {
		t.Fatalf("outcome = %q, diagnostics = %#v", result.Outcome, result.Diagnostics)
	}
	if !reflect.DeepEqual(result.Assignments.Categories, []classification.CategoryAssignment{
		{CategoryID: "visual"},
		{CategoryID: "wallpaper"},
	}) {
		t.Fatalf("categories = %#v", result.Assignments.Categories)
	}
	if result.Assignments.PrimaryCategoryID != "wallpaper" {
		t.Fatalf("primary = %q", result.Assignments.PrimaryCategoryID)
	}
}

func TestClassifyRejectsUnknownCategory(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  12,
		Categories: []classification.Category{
			{ID: "wallpaper", Slug: "wallpaper", Name: "壁纸", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1},
		},
	})

	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{
		PolicyKey:   "gallery.image.public",
		CategoryIDs: []string{"missing"},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.ClassifyFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if result.Diagnostics[0].Code != classification.DiagnosticCategoryUnknown {
		t.Fatalf("diagnostic = %#v", result.Diagnostics[0])
	}
	if result.Diagnostics[0].Reference != "missing" {
		t.Fatalf("reference = %q", result.Diagnostics[0].Reference)
	}
}

func TestClassifyRejectsCategoryWithInactiveAncestor(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  13,
		Categories: []classification.Category{
			{ID: "visual", Slug: "visual", Name: "视觉作品", Status: classification.StatusInactive},
			{ID: "wallpaper", ParentID: "visual", Slug: "wallpaper", Name: "壁纸", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1},
		},
	})

	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{
		PolicyKey:   "gallery.image.public",
		CategoryIDs: []string{"wallpaper"},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.ClassifyFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticCategoryInactive {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if result.Diagnostics[0].Params["reason"] != "ancestor_inactive" {
		t.Fatalf("diagnostic params = %#v", result.Diagnostics[0].Params)
	}
}

func TestClassifyRequiresPrimaryCategoryFromPolicy(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  14,
		Categories: []classification.Category{
			{ID: "wallpaper", Slug: "wallpaper", Name: "壁纸", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{
				Key:            "gallery.image.public",
				SchemaVersion:  1,
				PolicyRevision: 1,
				Category: classification.CategoryPolicy{
					MinAssignments: 1,
					MaxAssignments: 3,
					RequirePrimary: true,
				},
			},
		},
	})

	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{
		PolicyKey:   "gallery.image.public",
		CategoryIDs: []string{"wallpaper"},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.ClassifyFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticPrimaryCategoryRequired {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestClassifyUsesFacetCardinalityWithoutSelectionMode(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  15,
		Facets: []classification.Facet{
			{ID: "scene", Slug: "scene", Name: "场景", Status: classification.StatusActive},
		},
		FacetValues: []classification.FacetValue{
			{ID: "outdoor", FacetID: "scene", Slug: "outdoor", Name: "户外", Status: classification.StatusActive},
			{ID: "landscape", FacetID: "scene", ParentID: "outdoor", Slug: "landscape", Name: "风景", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{
				Key:            "gallery.image.public",
				SchemaVersion:  1,
				PolicyRevision: 1,
				Facets: []classification.FacetAssignmentPolicy{
					{FacetID: "scene", MinValues: 0, MaxValues: 2},
				},
			},
		},
	})

	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{
		PolicyKey: "gallery.image.public",
		Facets: []classification.FacetSelection{
			{FacetID: "scene", ValueIDs: []string{"outdoor", "landscape", "outdoor"}},
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.ClassifyFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeAccepted {
		t.Fatalf("outcome = %q, diagnostics = %#v", result.Outcome, result.Diagnostics)
	}
	if !reflect.DeepEqual(result.Assignments.Facets, []classification.FacetValueAssignment{
		{FacetID: "scene", ValueID: "landscape"},
		{FacetID: "scene", ValueID: "outdoor"},
	}) {
		t.Fatalf("facets = %#v", result.Assignments.Facets)
	}
}

func TestClassifyEnforcesFacetMaximumFromPolicy(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  16,
		Facets: []classification.Facet{
			{ID: "scene", Slug: "scene", Name: "场景", Status: classification.StatusActive},
		},
		FacetValues: []classification.FacetValue{
			{ID: "landscape", FacetID: "scene", Slug: "landscape", Name: "风景", Status: classification.StatusActive},
			{ID: "city", FacetID: "scene", Slug: "city", Name: "城市", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{
				Key:            "gallery.image.public",
				SchemaVersion:  1,
				PolicyRevision: 1,
				Facets: []classification.FacetAssignmentPolicy{
					{FacetID: "scene", MinValues: 0, MaxValues: 1},
				},
			},
		},
	})

	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{
		PolicyKey: "gallery.image.public",
		Facets: []classification.FacetSelection{
			{FacetID: "scene", ValueIDs: []string{"landscape", "city"}},
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.ClassifyFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticFacetTooManyValues {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if result.Diagnostics[0].Reference != "scene" {
		t.Fatalf("reference = %q", result.Diagnostics[0].Reference)
	}
}

func TestClassifyResolvesAliasAndProposesUnknownTagThroughFacts(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  17,
		Policies: []classification.PolicyProfile{
			{
				Key:            "gallery.image.public",
				SchemaVersion:  1,
				PolicyRevision: 1,
				Tags: classification.TagAdmissionPolicy{
					Unknown: classification.UnknownTagPropose,
				},
			},
		},
	})

	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{
		PolicyKey: "gallery.image.public",
		Tags:      []string{" ＮＩＧＨＴ ", "night", "  未知  "},
	})
	factRequest := preparation.FactRequest()
	if !reflect.DeepEqual(factRequest.TagLookups, []classification.TagLookupRequest{
		{LookupKey: "night", DisplayValue: "ＮＩＧＨＴ"},
		{LookupKey: "未知", DisplayValue: "未知"},
	}) {
		t.Fatalf("tag lookups = %#v", factRequest.TagLookups)
	}

	result := preparation.Complete(classification.ClassifyFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
		FreshnessToken:  "tags:5",
		TagMatches: []classification.TagMatch{
			{LookupKey: "night", Kind: classification.TagMatchAlias, TagID: "tag-night"},
			{LookupKey: "未知", Kind: classification.TagMatchNotFound},
		},
	})

	if result.Outcome != classification.OutcomeAccepted {
		t.Fatalf("outcome = %q, diagnostics = %#v", result.Outcome, result.Diagnostics)
	}
	if !reflect.DeepEqual(result.Assignments.Tags, []classification.TagAssignment{
		{TagID: "tag-night"},
	}) {
		t.Fatalf("tag assignments = %#v", result.Assignments.Tags)
	}
	if !reflect.DeepEqual(result.TagProposals, []classification.TagProposalInput{
		{LookupKey: "未知", DisplayValue: "未知"},
	}) {
		t.Fatalf("tag proposals = %#v", result.TagProposals)
	}
}

func TestClassifyRejectsInactiveTagInsteadOfProposingItAgain(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery", Revision: 18,
		Policies: []classification.PolicyProfile{{
			Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1,
			Tags: classification.TagAdmissionPolicy{Unknown: classification.UnknownTagPropose},
		}},
	})
	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{PolicyKey: "gallery.image.public", Tags: []string{"旧标签"}})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.ClassifyFacts{
		CatalogRevision: factRequest.CatalogRevision, RequestToken: factRequest.RequestToken, FreshnessToken: "tags:18",
		TagMatches: []classification.TagMatch{{LookupKey: "旧标签", Kind: classification.TagMatchInactive, TagID: "tag-old"}},
	})
	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticTagInactive {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestClassifyEnforcesTagAssignmentMaximum(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery", Revision: 19,
		Policies: []classification.PolicyProfile{{
			Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1,
			Tags: classification.TagAdmissionPolicy{Unknown: classification.UnknownTagReject, MaxAssignments: 1},
		}},
	})
	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{PolicyKey: "gallery.image.public", Tags: []string{"夜景", "雨"}})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.ClassifyFacts{
		CatalogRevision: factRequest.CatalogRevision, RequestToken: factRequest.RequestToken, FreshnessToken: "tags:19",
		TagMatches: []classification.TagMatch{
			{LookupKey: "夜景", Kind: classification.TagMatchCanonical, TagID: "tag-night"},
			{LookupKey: "雨", Kind: classification.TagMatchCanonical, TagID: "tag-rain"},
		},
	})
	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticTagTooManyAssignments {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestClassifyRejectsUnknownPolicyKey(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  18,
		Policies: []classification.PolicyProfile{
			{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1},
		},
	})

	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{
		PolicyKey: "gallery.image.missing",
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.ClassifyFacts{
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

func TestClassifyRequiresPrimaryToBeAnExplicitAssignment(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  19,
		Categories: []classification.Category{
			{ID: "wallpaper", Slug: "wallpaper", Name: "壁纸", Status: classification.StatusActive},
			{ID: "illustration", Slug: "illustration", Name: "插画", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{
				Key:            "gallery.image.public",
				SchemaVersion:  1,
				PolicyRevision: 1,
				Category: classification.CategoryPolicy{
					MinAssignments: 1,
					MaxAssignments: 3,
					RequirePrimary: true,
				},
			},
		},
	})

	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{
		PolicyKey:         "gallery.image.public",
		CategoryIDs:       []string{"wallpaper"},
		PrimaryCategoryID: "illustration",
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.ClassifyFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticPrimaryCategoryNotAssigned {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestClassifyRejectsFacetValueFromWrongFacet(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  20,
		Facets: []classification.Facet{
			{ID: "scene", Slug: "scene", Name: "场景", Status: classification.StatusActive},
			{ID: "style", Slug: "style", Name: "风格", Status: classification.StatusActive},
		},
		FacetValues: []classification.FacetValue{
			{ID: "minimal", FacetID: "style", Slug: "minimal", Name: "极简", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{
				Key:            "gallery.image.public",
				SchemaVersion:  1,
				PolicyRevision: 1,
				Facets: []classification.FacetAssignmentPolicy{
					{FacetID: "scene", MinValues: 0, MaxValues: 2},
				},
			},
		},
	})

	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{
		PolicyKey: "gallery.image.public",
		Facets: []classification.FacetSelection{
			{FacetID: "scene", ValueIDs: []string{"minimal"}},
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.ClassifyFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticFacetValueWrongFacet {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestClassifyEnforcesCategoryAndFacetMinimums(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery", Revision: 21,
		Facets: []classification.Facet{
			{ID: "scene", Slug: "scene", Name: "场景", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{
				Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1,
				Category: classification.CategoryPolicy{MinAssignments: 1},
				Facets:   []classification.FacetAssignmentPolicy{{FacetID: "scene", MinValues: 1}},
			},
		},
	})
	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{PolicyKey: "gallery.image.public"})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.ClassifyFacts{
		CatalogRevision: factRequest.CatalogRevision, RequestToken: factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if !reflect.DeepEqual(diagnosticCodes(result.Diagnostics), []classification.DiagnosticCode{
		classification.DiagnosticCategoryTooFewAssignments,
		classification.DiagnosticFacetTooFewValues,
	}) {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestClassifyEnforcesLeafAndDepthPolicies(t *testing.T) {
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery", Revision: 22,
		Categories: []classification.Category{
			{ID: "visual", Slug: "visual", Name: "视觉", Status: classification.StatusActive},
			{ID: "wallpaper", ParentID: "visual", Slug: "wallpaper", Name: "壁纸", Status: classification.StatusActive},
		},
		Facets: []classification.Facet{
			{ID: "scene", Slug: "scene", Name: "场景", Status: classification.StatusActive},
		},
		FacetValues: []classification.FacetValue{
			{ID: "outdoor", FacetID: "scene", Slug: "outdoor", Name: "户外", Status: classification.StatusActive},
			{ID: "landscape", FacetID: "scene", ParentID: "outdoor", Slug: "landscape", Name: "风景", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{
				Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1,
				Category: classification.CategoryPolicy{LeafOnly: true, MaxDepth: 1},
				Facets:   []classification.FacetAssignmentPolicy{{FacetID: "scene", LeafOnly: true, MaxDepth: 1}},
			},
		},
	})
	preparation := compiled.Catalog.Classify(classification.ClassifyRequest{
		PolicyKey: "gallery.image.public", CategoryIDs: []string{"visual", "wallpaper"},
		Facets: []classification.FacetSelection{{FacetID: "scene", ValueIDs: []string{"outdoor", "landscape"}}},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.ClassifyFacts{
		CatalogRevision: factRequest.CatalogRevision, RequestToken: factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if !reflect.DeepEqual(diagnosticCodes(result.Diagnostics), []classification.DiagnosticCode{
		classification.DiagnosticCategoryLeafRequired,
		classification.DiagnosticCategoryDepthExceeded,
		classification.DiagnosticFacetValueDepthExceeded,
		classification.DiagnosticFacetValueLeafRequired,
	}) {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func diagnosticCodes(values []classification.Diagnostic) []classification.DiagnosticCode {
	result := make([]classification.DiagnosticCode, 0, len(values))
	for _, value := range values {
		result = append(result, value.Code)
	}
	return result
}
