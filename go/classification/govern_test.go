package classification_test

import (
	"reflect"
	"testing"

	"github.com/yueli-official/foundation/go/classification"
)

func TestGovernRejectsCategoryReparentIntoOwnSubtree(t *testing.T) {
	catalog := mustGovernCatalog(t)
	preparation := catalog.Govern(classification.GovernRequest{
		PolicyKey: "gallery.image.public",
		Command: classification.GovernCommand{
			Operation: classification.GovernReparent,
			Kind:      classification.GovernCategory,
			ID:        "visual",
			ParentID:  "wallpaper",
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.GovernFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticGovernCycle {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestGovernRejectsReparentingReplacedIdentity(t *testing.T) {
	catalog := mustGovernCatalog(t)
	preparation := catalog.Govern(classification.GovernRequest{
		PolicyKey: "gallery.image.public",
		Command: classification.GovernCommand{
			Operation: classification.GovernReparent,
			Kind:      classification.GovernCategory,
			ID:        "retired",
			ParentID:  "visual",
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.GovernFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})
	if result.Outcome != classification.OutcomeRejected || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticGovernStatusTransitionInvalid {
		t.Fatalf("result = %#v", result)
	}
}

func TestGovernRejectsDraftFacetBecomingInactive(t *testing.T) {
	catalog := mustGovernCatalog(t)
	preparation := catalog.Govern(classification.GovernRequest{
		PolicyKey: "gallery.image.public",
		Command: classification.GovernCommand{
			Operation: classification.GovernSetStatus,
			Kind:      classification.GovernFacet,
			ID:        "draft-axis",
			Status:    classification.StatusInactive,
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.GovernFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})
	if result.Outcome != classification.OutcomeRejected || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticGovernStatusTransitionInvalid {
		t.Fatalf("result = %#v", result)
	}
}

func TestGovernDeleteRequiresExplicitDeleteAllRelatedAndProducesPlan(t *testing.T) {
	catalog := mustGovernCatalog(t)
	request := classification.GovernRequest{
		PolicyKey: "gallery.image.public",
		Command: classification.GovernCommand{
			Operation: classification.GovernDelete,
			Kind:      classification.GovernCategory,
			ID:        "wallpaper",
		},
	}
	preparation := catalog.Govern(request)
	factRequest := preparation.FactRequest()
	if !reflect.DeepEqual(factRequest.Impacts, []classification.ImpactRequest{
		{Kind: classification.GovernCategory, ID: "wallpaper"},
	}) {
		t.Fatalf("impact requests = %#v", factRequest.Impacts)
	}
	facts := classification.GovernFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
		FreshnessToken:  "impact:7",
		Impacts: []classification.ReferenceImpact{
			{Kind: classification.GovernCategory, ID: "wallpaper", AssignmentCount: 12, PrimaryCount: 7},
		},
	}
	rejected := preparation.Complete(facts)
	if rejected.Outcome != classification.OutcomeRejected || len(rejected.Diagnostics) != 1 || rejected.Diagnostics[0].Code != classification.DiagnosticGovernDeleteConfirmationRequired {
		t.Fatalf("rejected = %#v", rejected)
	}

	request.Command.DeleteAllRelated = true
	preparation = catalog.Govern(request)
	factRequest = preparation.FactRequest()
	facts.RequestToken = factRequest.RequestToken
	planned := preparation.Complete(facts)
	if planned.Outcome != classification.OutcomePlanned {
		t.Fatalf("outcome = %q, diagnostics = %#v", planned.Outcome, planned.Diagnostics)
	}
	if planned.Plan.ExpectedCatalogRevision != 31 || planned.Plan.ExpectedRequestToken == "" || planned.Plan.ExpectedImpactToken != "impact:7" {
		t.Fatalf("plan envelope = %#v", planned.Plan)
	}
	if !reflect.DeepEqual(planned.Plan.Steps, []classification.GovernStep{
		{Kind: classification.GovernClearPrimaryAssignments, IdentityKind: classification.GovernCategory, SourceID: "wallpaper", AffectedCount: 7},
		{Kind: classification.GovernDeleteAssignments, IdentityKind: classification.GovernCategory, SourceID: "wallpaper", AffectedCount: 12},
		{Kind: classification.GovernDeleteIdentity, IdentityKind: classification.GovernCategory, SourceID: "wallpaper"},
	}) {
		t.Fatalf("steps = %#v", planned.Plan.Steps)
	}
}

func TestGovernMergeRequiresCompleteExplicitChildPlan(t *testing.T) {
	catalog := mustGovernCatalog(t)
	preparation := catalog.Govern(classification.GovernRequest{
		PolicyKey: "gallery.image.public",
		Command: classification.GovernCommand{
			Operation: classification.GovernMerge,
			Kind:      classification.GovernCategory,
			ID:        "visual",
			TargetID:  "photography",
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.GovernFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
		FreshnessToken:  "impact:8",
		Impacts: []classification.ReferenceImpact{
			{Kind: classification.GovernCategory, ID: "visual", DirectChildIDs: []string{"wallpaper"}},
			{Kind: classification.GovernCategory, ID: "photography"},
		},
	})
	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticGovernChildPlanIncomplete {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestGovernRejectsFacetMergeUntilValueMappingHasAnExplicitContract(t *testing.T) {
	catalog := mustGovernCatalog(t)
	preparation := catalog.Govern(classification.GovernRequest{
		PolicyKey: "gallery.image.public",
		Command: classification.GovernCommand{
			Operation: classification.GovernMerge,
			Kind:      classification.GovernFacet,
			ID:        "scene",
			TargetID:  "style",
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.GovernFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticGovernOperationUnsupported {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestGovernRejectsInactiveMergeTarget(t *testing.T) {
	catalog := mustGovernCatalog(t)
	preparation := catalog.Govern(classification.GovernRequest{
		PolicyKey: "gallery.image.public",
		Command: classification.GovernCommand{
			Operation: classification.GovernMerge,
			Kind:      classification.GovernCategory,
			ID:        "photography",
			TargetID:  "archive",
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.GovernFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
		FreshnessToken:  "impact:inactive-target",
		Impacts: []classification.ReferenceImpact{
			{Kind: classification.GovernCategory, ID: "photography", Exists: true, Status: classification.StatusActive},
			{Kind: classification.GovernCategory, ID: "archive", Exists: true, Status: classification.StatusInactive},
		},
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticGovernStatusTransitionInvalid {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestGovernRejectsTagStatusOutsideActiveInactive(t *testing.T) {
	catalog := mustGovernCatalog(t)
	preparation := catalog.Govern(classification.GovernRequest{
		PolicyKey: "gallery.image.public",
		Command: classification.GovernCommand{
			Operation: classification.GovernSetStatus,
			Kind:      classification.GovernTag,
			ID:        "tag-1",
			Status:    classification.StatusReplaced,
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.GovernFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
		FreshnessToken:  "impact:tag",
		Impacts: []classification.ReferenceImpact{
			{Kind: classification.GovernTag, ID: "tag-1", Exists: true, Status: classification.StatusActive},
		},
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticGovernStatusTransitionInvalid {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestGovernRejectsMergeChildPlanThatCreatesCycle(t *testing.T) {
	catalog := mustGovernCatalog(t)
	preparation := catalog.Govern(classification.GovernRequest{
		PolicyKey: "gallery.image.public",
		Command: classification.GovernCommand{
			Operation: classification.GovernMerge,
			Kind:      classification.GovernCategory,
			ID:        "visual",
			TargetID:  "photography",
			ChildPlan: []classification.ChildMove{{ChildID: "wallpaper", ParentID: "wallpaper"}},
		},
	})
	factRequest := preparation.FactRequest()
	result := preparation.Complete(classification.GovernFacts{
		CatalogRevision: factRequest.CatalogRevision,
		RequestToken:    factRequest.RequestToken,
		FreshnessToken:  "impact:cycle",
		Impacts: []classification.ReferenceImpact{
			{Kind: classification.GovernCategory, ID: "visual", Exists: true, Status: classification.StatusActive, DirectChildIDs: []string{"wallpaper"}},
			{Kind: classification.GovernCategory, ID: "photography", Exists: true, Status: classification.StatusActive},
		},
	})

	if result.Outcome != classification.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != classification.DiagnosticGovernCycle {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func mustGovernCatalog(t *testing.T) *classification.Catalog {
	t.Helper()
	compiled := classification.Compile(classification.Snapshot{
		CatalogID: "gallery", Revision: 31,
		Categories: []classification.Category{
			{ID: "visual", Slug: "visual", Name: "视觉", Status: classification.StatusActive},
			{ID: "wallpaper", ParentID: "visual", Slug: "wallpaper", Name: "壁纸", Status: classification.StatusActive},
			{ID: "photography", Slug: "photography", Name: "摄影", Status: classification.StatusActive},
			{ID: "archive", Slug: "archive", Name: "归档", Status: classification.StatusInactive},
			{ID: "retired", Slug: "retired", Name: "旧分类", Status: classification.StatusReplaced, ReplacementID: "photography"},
		},
		Facets: []classification.Facet{
			{ID: "scene", Slug: "scene", Name: "场景", Status: classification.StatusActive},
			{ID: "style", Slug: "style", Name: "风格", Status: classification.StatusActive},
			{ID: "draft-axis", Slug: "draft-axis", Name: "草稿轴", Status: classification.StatusDraft},
		},
		Policies: []classification.PolicyProfile{
			{Key: "gallery.image.public", SchemaVersion: 1, PolicyRevision: 1},
		},
	})
	if compiled.Outcome != classification.OutcomeAccepted {
		t.Fatalf("compile = %#v", compiled)
	}
	return compiled.Catalog
}
