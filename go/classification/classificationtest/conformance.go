// Package classificationtest provides representative consumer policy fixtures
// and a public behavior contract for the Classification Module.
package classificationtest

import (
	"testing"

	"github.com/yueli-official/foundation/go/classification"
)

const (
	GalleryPolicyKey  = "gallery.image.public"
	BlogPolicyKey     = "blog.post.default"
	ResourcePolicyKey = "resource.item.default"
)

// Run verifies that the shared model supports the deliberately different
// Gallery, Blog and Resource policy shapes without consumer-specific branches.
func Run(t *testing.T) {
	t.Helper()

	t.Run("gallery-primary-facet-and-proposed-tag", func(t *testing.T) {
		catalog := compile(t, GallerySnapshot())
		preparation := catalog.Classify(classification.ClassifyRequest{
			PolicyKey:         GalleryPolicyKey,
			CategoryIDs:       []string{"gallery.category.wallpaper"},
			PrimaryCategoryID: "gallery.category.wallpaper",
			Facets: []classification.FacetSelection{{
				FacetID:  "gallery.facet.scene",
				ValueIDs: []string{"gallery.scene.landscape"},
			}},
			Tags: []string{"Rainy Night"},
		})
		request := preparation.FactRequest()
		result := preparation.Complete(classification.ClassifyFacts{
			CatalogRevision: request.CatalogRevision,
			RequestToken:    request.RequestToken,
			FreshnessToken:  "gallery-tags:7",
			TagMatches: []classification.TagMatch{{
				LookupKey: request.TagLookups[0].LookupKey,
				Kind:      classification.TagMatchNotFound,
			}},
		})
		if result.Outcome != classification.OutcomeAccepted || len(result.TagProposals) != 1 {
			t.Fatalf("Gallery Classify() = %#v", result)
		}
	})

	t.Run("blog-parent-filter-expands-descendants", func(t *testing.T) {
		catalog := compile(t, BlogSnapshot())
		preparation := catalog.Discover(classification.DiscoverRequest{
			PolicyKey: BlogPolicyKey,
			Categories: []classification.Reference{{
				Kind: classification.ReferenceBySlug, Value: "engineering",
			}},
		})
		request := preparation.FactRequest()
		result := preparation.Complete(classification.DiscoverFacts{
			CatalogRevision: request.CatalogRevision,
			RequestToken:    request.RequestToken,
		})
		if result.Outcome != classification.OutcomeAccepted ||
			len(result.FilterPlan.Groups) != 1 ||
			len(result.FilterPlan.Groups[0].ValueIDs) != 2 {
			t.Fatalf("Blog Discover() = %#v", result)
		}
	})

	t.Run("resource-rejects-unregistered-tag", func(t *testing.T) {
		catalog := compile(t, ResourceSnapshot())
		preparation := catalog.Classify(classification.ClassifyRequest{
			PolicyKey:   ResourcePolicyKey,
			CategoryIDs: []string{"resource.category.tools"},
			Tags:        []string{"unregistered"},
		})
		request := preparation.FactRequest()
		result := preparation.Complete(classification.ClassifyFacts{
			CatalogRevision: request.CatalogRevision,
			RequestToken:    request.RequestToken,
			FreshnessToken:  "resource-tags:3",
			TagMatches: []classification.TagMatch{{
				LookupKey: request.TagLookups[0].LookupKey,
				Kind:      classification.TagMatchNotFound,
			}},
		})
		if result.Outcome != classification.OutcomeRejected ||
			!containsDiagnostic(result.Diagnostics, classification.DiagnosticTagRejected) {
			t.Fatalf("Resource Classify() = %#v", result)
		}
	})
}

func compile(t *testing.T, snapshot classification.Snapshot) *classification.Catalog {
	t.Helper()
	result := classification.Compile(snapshot)
	if result.Outcome != classification.OutcomeAccepted || result.Catalog == nil {
		t.Fatalf("Compile() = %#v", result)
	}
	return result.Catalog
}

func containsDiagnostic(values []classification.Diagnostic, code classification.DiagnosticCode) bool {
	for _, value := range values {
		if value.Code == code {
			return true
		}
	}
	return false
}
