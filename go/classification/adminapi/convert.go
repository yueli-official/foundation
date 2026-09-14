package adminapi

import "github.com/yueli-official/foundation/go/classification"

func FromCategory(value classification.Category, revision Revision) Category {
	return Category{
		ID:                value.ID,
		ParentID:          value.ParentID,
		Slug:              value.Slug,
		Name:              value.Name,
		Status:            value.Status,
		Revision:          revision,
		EditorialPosition: cloneInt(value.EditorialPosition),
		ReplacementID:     value.ReplacementID,
	}
}

func (value Category) Domain() classification.Category {
	return classification.Category{
		ID:                value.ID,
		ParentID:          value.ParentID,
		Slug:              value.Slug,
		Name:              value.Name,
		Status:            value.Status,
		EditorialPosition: cloneInt(value.EditorialPosition),
		ReplacementID:     value.ReplacementID,
	}
}

func FromFacet(value classification.Facet, revision Revision) Facet {
	return Facet{
		ID:                value.ID,
		Slug:              value.Slug,
		Name:              value.Name,
		Status:            value.Status,
		Revision:          revision,
		EditorialPosition: cloneInt(value.EditorialPosition),
		ReplacementID:     value.ReplacementID,
	}
}

func (value Facet) Domain() classification.Facet {
	return classification.Facet{
		ID:                value.ID,
		Slug:              value.Slug,
		Name:              value.Name,
		Status:            value.Status,
		EditorialPosition: cloneInt(value.EditorialPosition),
		ReplacementID:     value.ReplacementID,
	}
}

func FromFacetValue(value classification.FacetValue, revision Revision) FacetValue {
	return FacetValue{
		ID:                value.ID,
		FacetID:           value.FacetID,
		ParentID:          value.ParentID,
		Slug:              value.Slug,
		Name:              value.Name,
		Status:            value.Status,
		Revision:          revision,
		EditorialPosition: cloneInt(value.EditorialPosition),
		ReplacementID:     value.ReplacementID,
	}
}

func (value FacetValue) Domain() classification.FacetValue {
	return classification.FacetValue{
		ID:                value.ID,
		FacetID:           value.FacetID,
		ParentID:          value.ParentID,
		Slug:              value.Slug,
		Name:              value.Name,
		Status:            value.Status,
		EditorialPosition: cloneInt(value.EditorialPosition),
		ReplacementID:     value.ReplacementID,
	}
}

func FromPolicyProfile(value classification.PolicyProfile) PolicyProfile {
	facets := make([]FacetAssignmentPolicy, len(value.Facets))
	for index, policy := range value.Facets {
		facets[index] = FacetAssignmentPolicy{
			FacetID:   policy.FacetID,
			MinValues: policy.MinValues,
			MaxValues: policy.MaxValues,
			LeafOnly:  policy.LeafOnly,
			MaxDepth:  policy.MaxDepth,
		}
	}
	return PolicyProfile{
		Key:           value.Key,
		SchemaVersion: value.SchemaVersion,
		Revision:      Revision(value.PolicyRevision),
		Category: CategoryPolicy{
			MinAssignments: value.Category.MinAssignments,
			MaxAssignments: value.Category.MaxAssignments,
			RequirePrimary: value.Category.RequirePrimary,
			LeafOnly:       value.Category.LeafOnly,
			MaxDepth:       value.Category.MaxDepth,
		},
		Facets: facets,
		Tags: TagAdmissionPolicy{
			Unknown:        value.Tags.Unknown,
			MinAssignments: value.Tags.MinAssignments,
			MaxAssignments: value.Tags.MaxAssignments,
		},
		Discovery: DiscoveryPolicy{DefaultSort: value.Discovery.DefaultSort},
	}
}

func (value PolicyProfile) Domain() classification.PolicyProfile {
	facets := make([]classification.FacetAssignmentPolicy, len(value.Facets))
	for index, policy := range value.Facets {
		facets[index] = classification.FacetAssignmentPolicy{
			FacetID:   policy.FacetID,
			MinValues: policy.MinValues,
			MaxValues: policy.MaxValues,
			LeafOnly:  policy.LeafOnly,
			MaxDepth:  policy.MaxDepth,
		}
	}
	return classification.PolicyProfile{
		Key:            value.Key,
		SchemaVersion:  value.SchemaVersion,
		PolicyRevision: uint64(value.Revision),
		Category: classification.CategoryPolicy{
			MinAssignments: value.Category.MinAssignments,
			MaxAssignments: value.Category.MaxAssignments,
			RequirePrimary: value.Category.RequirePrimary,
			LeafOnly:       value.Category.LeafOnly,
			MaxDepth:       value.Category.MaxDepth,
		},
		Facets: facets,
		Tags: classification.TagAdmissionPolicy{
			Unknown:        value.Tags.Unknown,
			MinAssignments: value.Tags.MinAssignments,
			MaxAssignments: value.Tags.MaxAssignments,
		},
		Discovery: classification.DiscoveryPolicy{DefaultSort: value.Discovery.DefaultSort},
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
