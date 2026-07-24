package classificationtest

import "github.com/yueli-official/foundation/go/classification"

// GallerySnapshot represents a curated image catalog with mandatory Primary
// Category, a controlled Facet and moderated unknown Tags.
func GallerySnapshot() classification.Snapshot {
	return classification.Snapshot{
		CatalogID: "gallery",
		Revision:  1,
		Categories: []classification.Category{{
			ID: "gallery.category.wallpaper", Slug: "wallpaper", Name: "Wallpaper",
			Status: classification.StatusActive,
		}},
		Facets: []classification.Facet{{
			ID: "gallery.facet.scene", Slug: "scene", Name: "Scene",
			Status: classification.StatusActive,
		}},
		FacetValues: []classification.FacetValue{{
			ID: "gallery.scene.landscape", FacetID: "gallery.facet.scene",
			Slug: "landscape", Name: "Landscape", Status: classification.StatusActive,
		}},
		Policies: []classification.PolicyProfile{{
			Key: GalleryPolicyKey, SchemaVersion: 1, PolicyRevision: 1,
			Category: classification.CategoryPolicy{
				MinAssignments: 1, MaxAssignments: 3, RequirePrimary: true,
			},
			Facets: []classification.FacetAssignmentPolicy{{
				FacetID: "gallery.facet.scene", MinValues: 1, MaxValues: 2,
			}},
			Tags: classification.TagAdmissionPolicy{
				Unknown: classification.UnknownTagPropose, MaxAssignments: 20,
			},
			Discovery: classification.DiscoveryPolicy{
				DefaultSort: classification.CandidateSortEditorial,
			},
		}},
	}
}

// BlogSnapshot represents hierarchical, multi-assignment editorial Categories
// with no Facets or Primary Category and direct unknown Tag creation.
func BlogSnapshot() classification.Snapshot {
	return classification.Snapshot{
		CatalogID: "blog",
		Revision:  1,
		Categories: []classification.Category{
			{
				ID: "blog.category.engineering", Slug: "engineering",
				Name: "Engineering", Status: classification.StatusActive,
			},
			{
				ID: "blog.category.backend", ParentID: "blog.category.engineering",
				Slug: "backend", Name: "Backend", Status: classification.StatusActive,
			},
		},
		Policies: []classification.PolicyProfile{{
			Key: BlogPolicyKey, SchemaVersion: 1, PolicyRevision: 1,
			Category: classification.CategoryPolicy{MaxAssignments: 8},
			Tags: classification.TagAdmissionPolicy{
				Unknown: classification.UnknownTagCreate, MaxAssignments: 20,
			},
			Discovery: classification.DiscoveryPolicy{
				DefaultSort: classification.CandidateSortNameAsc,
			},
		}},
	}
}

// ResourceSnapshot represents operator-managed Categories and Tags. Editors
// assign only existing Tags; unknown text is rejected instead of implicitly
// creating governance data.
func ResourceSnapshot() classification.Snapshot {
	return classification.Snapshot{
		CatalogID: "resource",
		Revision:  1,
		Categories: []classification.Category{{
			ID: "resource.category.tools", Slug: "tools", Name: "Tools",
			Status: classification.StatusActive,
		}},
		Policies: []classification.PolicyProfile{{
			Key: ResourcePolicyKey, SchemaVersion: 1, PolicyRevision: 1,
			Category: classification.CategoryPolicy{MaxAssignments: 8},
			Tags: classification.TagAdmissionPolicy{
				Unknown: classification.UnknownTagReject, MaxAssignments: 20,
			},
			Discovery: classification.DiscoveryPolicy{
				DefaultSort: classification.CandidateSortNameAsc,
			},
		}},
	}
}
