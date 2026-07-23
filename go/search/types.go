package search

import "time"

type (
	AnalyzerKey        string
	BatchID            string
	DocumentKind       string
	DocumentID         string
	FilterName         string
	GenerationID       string
	ProjectionRevision uint64
	Cursor             string
)

type DocumentKey struct {
	Kind DocumentKind `json:"kind"`
	ID   DocumentID   `json:"id"`
}

type VisibilityReference struct {
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
}

type Value struct {
	values []string
}

func Keyword(value string) Value      { return Value{values: []string{value}} }
func Keywords(values ...string) Value { return Value{values: append([]string(nil), values...)} }

type FieldValues map[FilterName]Value

type SourceDocument struct {
	Key        DocumentKey
	Revision   ProjectionRevision
	Analyzer   AnalyzerKey
	Title      string
	Summary    string
	Body       string
	Keywords   []string
	Filters    FieldValues
	SortAt     time.Time
	Visibility VisibilityReference
}

type changeKind uint8

const (
	changeUpsert changeKind = iota + 1
	changeRemove
)

type Change struct {
	kind     changeKind
	document SourceDocument
	key      DocumentKey
	revision ProjectionRevision
}

func Upsert(document SourceDocument) Change {
	return Change{kind: changeUpsert, key: document.Key, revision: document.Revision, document: document}
}

func Remove(key DocumentKey, revision ProjectionRevision) Change {
	return Change{kind: changeRemove, key: key, revision: revision}
}

type Batch struct {
	ID      BatchID
	Changes []Change
}

type ApplyResult struct {
	Applied int `json:"applied"`
	Replays int `json:"replays"`
	Stale   int `json:"stale"`
}

type TextField string

const (
	TextTitle   TextField = "title"
	TextSummary TextField = "summary"
	TextBody    TextField = "body"
)

type QueryMode string

const (
	QueryWeb   QueryMode = "web"
	QueryPlain QueryMode = "plain"
)

type Capability string

const (
	CapabilityFullText            Capability = "full_text"
	CapabilityPhrase              Capability = "phrase"
	CapabilityChineseSegmentation Capability = "chinese_segmentation"
)

type SortKind string

const (
	SortRelevance SortKind = "relevance"
	SortNewest    SortKind = "newest"
)

type Filter struct {
	name   FilterName
	values []string
	all    bool
}

func Any(name FilterName, values ...string) Filter {
	return Filter{name: name, values: append([]string(nil), values...)}
}

func Equal(name FilterName, value string) Filter { return Any(name, value) }

func All(name FilterName, values ...string) Filter {
	return Filter{name: name, values: append([]string(nil), values...), all: true}
}

type FacetRequest struct {
	Name  FilterName
	Limit int
}

type HighlightRequest struct {
	Fields       []TextField
	MaxFragments int
}

type PageRequest struct {
	Size   int
	Cursor Cursor
}

type Query struct {
	Text      string
	Analyzer  AnalyzerKey
	Filters   []Filter
	Facets    []FacetRequest
	Sort      SortKind
	Page      PageRequest
	Highlight HighlightRequest
}

type PlanSummary struct {
	DefinitionDigest string       `json:"definitionDigest"`
	Engine           string       `json:"engine"`
	Generation       GenerationID `json:"generation"`
	QueryDigest      string       `json:"queryDigest"`
	Sort             SortKind     `json:"sort"`
}

type Segment struct {
	Text    string `json:"text"`
	Matched bool   `json:"matched"`
}

type Fragment struct {
	Segments []Segment `json:"segments"`
}

type Highlight struct {
	Field     TextField  `json:"field"`
	Fragments []Fragment `json:"fragments"`
}

type Hit struct {
	Key        DocumentKey         `json:"key"`
	Revision   ProjectionRevision  `json:"revision"`
	Visibility VisibilityReference `json:"visibility"`
	Score      float32             `json:"score"`
	Highlights []Highlight         `json:"highlights,omitempty"`
}

type FacetBucket struct {
	Value string `json:"value"`
	Count uint64 `json:"count"`
}

type FacetResult struct {
	Name    FilterName    `json:"name"`
	Buckets []FacetBucket `json:"buckets"`
}

type Page struct {
	Plan       PlanSummary   `json:"plan"`
	Hits       []Hit         `json:"hits"`
	Total      uint64        `json:"total"`
	Facets     []FacetResult `json:"facets"`
	NextCursor Cursor        `json:"nextCursor,omitempty"`
}
