package classification_test

import (
	"testing"

	"github.com/yueli-official/foundation/go/classification"
)

func TestCompileAcceptsHierarchiesAndVersionedPolicies(t *testing.T) {
	result := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  7,
		Categories: []classification.Category{
			{ID: "visual", Slug: "visual", Name: "视觉作品", Status: classification.StatusActive},
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
				Key:            "gallery.image.public",
				SchemaVersion:  1,
				PolicyRevision: 3,
				Category: classification.CategoryPolicy{
					MinAssignments: 1,
					MaxAssignments: 3,
					RequirePrimary: true,
				},
				Facets: []classification.FacetAssignmentPolicy{
					{FacetID: "scene", MinValues: 0, MaxValues: 2},
				},
				Tags: classification.TagAdmissionPolicy{
					Unknown: classification.UnknownTagPropose,
				},
				Discovery: classification.DiscoveryPolicy{
					DefaultSort: classification.CandidateSortEditorial,
				},
			},
			{
				Key:            "blog.post.default",
				SchemaVersion:  1,
				PolicyRevision: 1,
				Category: classification.CategoryPolicy{
					MinAssignments: 0,
					MaxAssignments: 0,
				},
				Tags: classification.TagAdmissionPolicy{
					Unknown: classification.UnknownTagCreate,
				},
			},
		},
	})

	if result.Outcome != classification.OutcomeAccepted {
		t.Fatalf("outcome = %q, diagnostics = %#v", result.Outcome, result.Diagnostics)
	}
	if result.Catalog == nil {
		t.Fatal("accepted compile must return a catalog")
	}
	if result.CatalogRevision != 7 {
		t.Fatalf("catalog revision = %d", result.CatalogRevision)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestCompileRejectsCategoryCyclesWithTypedDiagnostic(t *testing.T) {
	result := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  8,
		Categories: []classification.Category{
			{ID: "wallpaper", ParentID: "illustration", Slug: "wallpaper", Name: "壁纸", Status: classification.StatusActive},
			{ID: "illustration", ParentID: "wallpaper", Slug: "illustration", Name: "插画", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1},
		},
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if result.Catalog != nil {
		t.Fatal("rejected compile must not return a catalog")
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if result.Diagnostics[0].Code != classification.DiagnosticCategoryCycle {
		t.Fatalf("diagnostic = %#v", result.Diagnostics[0])
	}
	if result.Diagnostics[0].Reference != "wallpaper" {
		t.Fatalf("reference = %q", result.Diagnostics[0].Reference)
	}
}

func TestCompileRejectsFacetValueParentFromAnotherFacet(t *testing.T) {
	result := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  9,
		Facets: []classification.Facet{
			{ID: "scene", Slug: "scene", Name: "场景", Status: classification.StatusActive},
			{ID: "style", Slug: "style", Name: "风格", Status: classification.StatusActive},
		},
		FacetValues: []classification.FacetValue{
			{ID: "outdoor", FacetID: "scene", Slug: "outdoor", Name: "户外", Status: classification.StatusActive},
			{ID: "minimal", FacetID: "style", ParentID: "outdoor", Slug: "minimal", Name: "极简", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1},
		},
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticFacetValueParentScope {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if result.Diagnostics[0].Reference != "minimal" {
		t.Fatalf("reference = %q", result.Diagnostics[0].Reference)
	}
}

func TestCompileRejectsFacetValueCycle(t *testing.T) {
	result := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  10,
		Facets: []classification.Facet{
			{ID: "scene", Slug: "scene", Name: "场景", Status: classification.StatusActive},
		},
		FacetValues: []classification.FacetValue{
			{ID: "outdoor", FacetID: "scene", ParentID: "landscape", Slug: "outdoor", Name: "户外", Status: classification.StatusActive},
			{ID: "landscape", FacetID: "scene", ParentID: "outdoor", Slug: "landscape", Name: "风景", Status: classification.StatusActive},
		},
		Policies: []classification.PolicyProfile{
			{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1},
		},
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticFacetValueCycle {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if result.Diagnostics[0].Reference != "outdoor" {
		t.Fatalf("reference = %q", result.Diagnostics[0].Reference)
	}
}

func TestCompileRejectsPolicyReferencingUnknownFacet(t *testing.T) {
	result := classification.Compile(classification.Snapshot{
		CatalogID: "gallery",
		Revision:  11,
		Policies: []classification.PolicyProfile{
			{
				Key:            "gallery.image.public",
				SchemaVersion:  1,
				PolicyRevision: 1,
				Facets: []classification.FacetAssignmentPolicy{
					{FacetID: "missing", MinValues: 0, MaxValues: 1},
				},
			},
		},
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticPolicyFacetUnknown {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if result.Diagnostics[0].Reference != "missing" {
		t.Fatalf("reference = %q", result.Diagnostics[0].Reference)
	}
}

func TestCompileRejectsInvalidPolicyRanges(t *testing.T) {
	result := classification.Compile(classification.Snapshot{
		CatalogID: "gallery", Revision: 12,
		Policies: []classification.PolicyProfile{
			{
				Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1,
				Category: classification.CategoryPolicy{MinAssignments: 3, MaxAssignments: 2},
			},
		},
	})
	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticPolicyRangeInvalid {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestCompileRejectsInvalidCatalogIdentities(t *testing.T) {
	tests := []struct {
		name     string
		snapshot classification.Snapshot
		code     classification.DiagnosticCode
	}{
		{
			name: "unknown category parent",
			snapshot: classification.Snapshot{
				CatalogID: "gallery", Revision: 13,
				Categories: []classification.Category{{ID: "wallpaper", ParentID: "missing", Slug: "wallpaper", Status: classification.StatusActive}},
			},
			code: classification.DiagnosticCategoryParentUnknown,
		},
		{
			name: "duplicate facet slug",
			snapshot: classification.Snapshot{
				CatalogID: "gallery", Revision: 13,
				Facets: []classification.Facet{
					{ID: "scene-1", Slug: "scene", Status: classification.StatusActive},
					{ID: "scene-2", Slug: "scene", Status: classification.StatusActive},
				},
			},
			code: classification.DiagnosticFacetDuplicateSlug,
		},
		{
			name: "unknown facet value owner",
			snapshot: classification.Snapshot{
				CatalogID: "gallery", Revision: 13,
				FacetValues: []classification.FacetValue{{ID: "landscape", FacetID: "missing", Slug: "landscape", Status: classification.StatusActive}},
			},
			code: classification.DiagnosticFacetValueFacetUnknown,
		},
		{
			name: "replacement chain",
			snapshot: classification.Snapshot{
				CatalogID: "gallery", Revision: 13,
				Categories: []classification.Category{
					{ID: "old", Slug: "old", Status: classification.StatusReplaced, ReplacementID: "middle"},
					{ID: "middle", Slug: "middle", Status: classification.StatusReplaced, ReplacementID: "new"},
					{ID: "new", Slug: "new", Status: classification.StatusActive},
				},
			},
			code: classification.DiagnosticCategoryReplacementInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := classification.Compile(test.snapshot)
			if result.Outcome != classification.OutcomeRejected {
				t.Fatalf("outcome = %q", result.Outcome)
			}
			if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != test.code {
				t.Fatalf("diagnostics = %#v", result.Diagnostics)
			}
		})
	}
}
