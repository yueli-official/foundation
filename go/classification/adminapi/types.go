// Package adminapi defines the framework-neutral HTTP/JSON contract used to
// administer Foundation Classification catalogs. Products own routing,
// persistence, authorization and transactions; this package owns the stable
// wire vocabulary shared by their adapters.
package adminapi

import "github.com/yueli-official/foundation/go/classification"

const SchemaVersion uint16 = 1

type Revision uint64

type Status = classification.Status

const (
	StatusDraft    = classification.StatusDraft
	StatusActive   = classification.StatusActive
	StatusInactive = classification.StatusInactive
	StatusReplaced = classification.StatusReplaced
)

// Category is the canonical admin representation of one Classification
// category. Product DTOs may embed Category and add product-owned presentation
// fields such as an icon, image reference or contextual usage count.
type Category struct {
	ID                string   `json:"id"`
	ParentID          string   `json:"parentId,omitempty"`
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Status            Status   `json:"status"`
	Revision          Revision `json:"revision"`
	EditorialPosition *int     `json:"editorialPosition,omitempty"`
	ReplacementID     string   `json:"replacementId,omitempty"`
}

// Facet is the canonical admin representation of one controlled filter axis.
// Assignment policy is kept separate because one catalog may expose different
// policies to different product scopes.
type Facet struct {
	ID                string   `json:"id"`
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Status            Status   `json:"status"`
	Revision          Revision `json:"revision"`
	EditorialPosition *int     `json:"editorialPosition,omitempty"`
	ReplacementID     string   `json:"replacementId,omitempty"`
}

// FacetValue always carries its owner Facet. parentId is meaningful only
// within the same Facet and products must reject cross-Facet parent changes.
type FacetValue struct {
	ID                string   `json:"id"`
	FacetID           string   `json:"facetId"`
	ParentID          string   `json:"parentId,omitempty"`
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Status            Status   `json:"status"`
	Revision          Revision `json:"revision"`
	EditorialPosition *int     `json:"editorialPosition,omitempty"`
	ReplacementID     string   `json:"replacementId,omitempty"`
}

// Tag is flat. Alias lookup and normalization remain consumer-owned registry
// behavior; this DTO represents the canonical tag identity managed by admins.
type Tag struct {
	ID                string   `json:"id"`
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Status            Status   `json:"status"`
	Revision          Revision `json:"revision"`
	EditorialPosition *int     `json:"editorialPosition,omitempty"`
	ReplacementID     string   `json:"replacementId,omitempty"`
}

// List is the canonical small-collection response shape. Use NewList when a
// source may return nil so an empty response encodes as [] rather than null.
type List[T any] struct {
	Items []T `json:"items"`
}

func NewList[T any](items []T) List[T] {
	if items == nil {
		items = []T{}
	}
	return List[T]{Items: items}
}

type CategoryMutation struct {
	ParentID          string   `json:"parentId,omitempty"`
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Status            Status   `json:"status"`
	Revision          Revision `json:"revision"`
	EditorialPosition *int     `json:"editorialPosition,omitempty"`
	ReplacementID     string   `json:"replacementId,omitempty"`
}

type FacetMutation struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Status            Status   `json:"status"`
	Revision          Revision `json:"revision"`
	EditorialPosition *int     `json:"editorialPosition,omitempty"`
	ReplacementID     string   `json:"replacementId,omitempty"`
}

type FacetValueMutation struct {
	ParentID          string   `json:"parentId,omitempty"`
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Status            Status   `json:"status"`
	Revision          Revision `json:"revision"`
	EditorialPosition *int     `json:"editorialPosition,omitempty"`
	ReplacementID     string   `json:"replacementId,omitempty"`
}

type TagMutation struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Status            Status   `json:"status"`
	Revision          Revision `json:"revision"`
	EditorialPosition *int     `json:"editorialPosition,omitempty"`
	ReplacementID     string   `json:"replacementId,omitempty"`
}

type DeleteMutation struct {
	Revision Revision `json:"revision"`
}

type CategoryPolicy struct {
	MinAssignments int  `json:"minAssignments"`
	MaxAssignments int  `json:"maxAssignments"`
	RequirePrimary bool `json:"requirePrimary"`
	LeafOnly       bool `json:"leafOnly"`
	MaxDepth       int  `json:"maxDepth"`
}

type FacetAssignmentPolicy struct {
	FacetID   string `json:"facetId"`
	MinValues int    `json:"minValues"`
	MaxValues int    `json:"maxValues"`
	LeafOnly  bool   `json:"leafOnly"`
	MaxDepth  int    `json:"maxDepth"`
}

type TagAdmissionPolicy struct {
	Unknown        classification.UnknownTagAdmission `json:"unknown"`
	MinAssignments int                                `json:"minAssignments"`
	MaxAssignments int                                `json:"maxAssignments"`
}

type DiscoveryPolicy struct {
	DefaultSort classification.CandidateSort `json:"defaultSort"`
}

// PolicyProfile is the wire projection of one domain PolicyProfile. revision
// is the policy revision, while schemaVersion selects the policy schema.
type PolicyProfile struct {
	Key           string                  `json:"key"`
	SchemaVersion uint16                  `json:"schemaVersion"`
	Revision      Revision                `json:"revision"`
	Category      CategoryPolicy          `json:"category"`
	Facets        []FacetAssignmentPolicy `json:"facets"`
	Tags          TagAdmissionPolicy      `json:"tags"`
	Discovery     DiscoveryPolicy         `json:"discovery"`
}

// CatalogSnapshot is the complete canonical admin projection when a product
// offers a single snapshot endpoint in addition to resource collection routes.
// Products with several classification scopes mount one snapshot per scope.
type CatalogSnapshot struct {
	SchemaVersion uint16          `json:"schemaVersion"`
	CatalogID     string          `json:"catalogId"`
	Revision      Revision        `json:"revision"`
	Categories    []Category      `json:"categories"`
	Facets        []Facet         `json:"facets"`
	FacetValues   []FacetValue    `json:"facetValues"`
	Tags          []Tag           `json:"tags"`
	Policies      []PolicyProfile `json:"policies"`
}

func NewCatalogSnapshot(catalogID string, revision Revision) CatalogSnapshot {
	return CatalogSnapshot{
		SchemaVersion: SchemaVersion,
		CatalogID:     catalogID,
		Revision:      revision,
		Categories:    []Category{},
		Facets:        []Facet{},
		FacetValues:   []FacetValue{},
		Tags:          []Tag{},
		Policies:      []PolicyProfile{},
	}
}
