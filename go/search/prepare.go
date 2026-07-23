package search

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type preparedChange struct {
	Kind     changeKind
	Key      DocumentKey
	Revision ProjectionRevision
	Document SourceDocument
	Filters  map[string][]string
	Digest   string
}

type preparedBatch struct {
	ID          BatchID
	Changes     []preparedChange
	Fingerprint string
}

func (catalog *Catalog) prepareBatch(batch Batch) (preparedBatch, error) {
	if !stableName.MatchString(string(batch.ID)) {
		return preparedBatch{}, &Error{Kind: ErrorInvalidDocument, Field: "batch.id", Message: "is invalid"}
	}
	if len(batch.Changes) == 0 || len(batch.Changes) > catalog.definition.Limits.MaxBatchDocuments {
		return preparedBatch{}, &Error{Kind: ErrorCapacity, Field: "batch.changes", Message: "is outside the allowed size"}
	}
	out := preparedBatch{ID: batch.ID, Changes: make([]preparedChange, 0, len(batch.Changes))}
	totalBytes := 0
	keys := make(map[DocumentKey]struct{}, len(batch.Changes))
	for index, change := range batch.Changes {
		if _, exists := keys[change.key]; exists {
			return preparedBatch{}, &Error{Kind: ErrorInvalidDocument, Field: fmt.Sprintf("changes[%d].key", index), Message: "is duplicated in the batch"}
		}
		keys[change.key] = struct{}{}
		prepared, bytes, err := catalog.prepareChange(change)
		if err != nil {
			if value, ok := err.(*Error); ok {
				value.Field = fmt.Sprintf("changes[%d].%s", index, value.Field)
			}
			return preparedBatch{}, err
		}
		totalBytes += bytes
		if totalBytes > catalog.definition.Limits.MaxBatchBytes {
			return preparedBatch{}, &Error{Kind: ErrorCapacity, Field: "batch", Message: "exceeds the byte budget"}
		}
		out.Changes = append(out.Changes, prepared)
	}
	encoded, _ := json.Marshal(out.Changes)
	sum := sha256.Sum256(encoded)
	out.Fingerprint = hex.EncodeToString(sum[:])
	return out, nil
}

func (catalog *Catalog) prepareChange(change Change) (preparedChange, int, error) {
	if change.kind != changeUpsert && change.kind != changeRemove {
		return preparedChange{}, 0, &Error{Kind: ErrorInvalidDocument, Field: "kind", Message: "is invalid"}
	}
	if !stableName.MatchString(string(change.key.Kind)) || strings.TrimSpace(string(change.key.ID)) == "" {
		return preparedChange{}, 0, &Error{Kind: ErrorInvalidDocument, Field: "key", Message: "is invalid"}
	}
	if change.revision == 0 {
		return preparedChange{}, 0, &Error{Kind: ErrorInvalidDocument, Field: "revision", Message: "must be positive"}
	}
	prepared := preparedChange{Kind: change.kind, Key: change.key, Revision: change.revision}
	if change.kind == changeRemove {
		encoded, _ := json.Marshal(prepared)
		sum := sha256.Sum256(encoded)
		prepared.Digest = hex.EncodeToString(sum[:])
		return prepared, len(encoded), nil
	}
	document := change.document
	if document.Key != change.key || document.Revision != change.revision {
		return preparedChange{}, 0, &Error{Kind: ErrorInvalidDocument, Field: "document", Message: "key or revision changed"}
	}
	if _, exists := catalog.analyzers[document.Analyzer]; !exists {
		return preparedChange{}, 0, &Error{Kind: ErrorInvalidDocument, Field: "analyzer", Message: "is not declared"}
	}
	if document.SortAt.IsZero() {
		return preparedChange{}, 0, &Error{Kind: ErrorInvalidDocument, Field: "sort_at", Message: "is required"}
	}
	if !stableName.MatchString(document.Visibility.ResourceType) ||
		strings.TrimSpace(document.Visibility.ResourceID) == "" {
		return preparedChange{}, 0, &Error{Kind: ErrorInvalidDocument, Field: "visibility", Message: "is invalid"}
	}
	document.SortAt = document.SortAt.UTC()
	document.Title = strings.TrimSpace(document.Title)
	var err error
	document.Keywords, err = normalizedStrings(document.Keywords, 100, 200)
	if err != nil {
		return preparedChange{}, 0, &Error{Kind: ErrorInvalidDocument, Field: "keywords", Message: err.Error()}
	}
	prepared.Filters = make(map[string][]string, len(document.Filters))
	for name, value := range document.Filters {
		definition, exists := catalog.filters[name]
		if !exists {
			return preparedChange{}, 0, &Error{Kind: ErrorInvalidDocument, Field: "filters." + string(name), Message: "is not declared"}
		}
		values, err := normalizedStrings(value.values, definition.MaxValues, definition.MaxBytes)
		if err != nil {
			return preparedChange{}, 0, &Error{Kind: ErrorInvalidDocument, Field: "filters." + string(name), Message: err.Error()}
		}
		prepared.Filters[string(name)] = values
	}
	document.Filters = nil
	prepared.Document = document
	encoded, _ := json.Marshal(struct {
		Document SourceDocument
		Filters  map[string][]string
	}{document, prepared.Filters})
	if len(encoded) > catalog.definition.Limits.MaxDocumentBytes {
		return preparedChange{}, 0, &Error{Kind: ErrorCapacity, Field: "document", Message: "exceeds the byte budget"}
	}
	sum := sha256.Sum256(encoded)
	prepared.Digest = hex.EncodeToString(sum[:])
	return prepared, len(encoded), nil
}

type preparedQuery struct {
	Text       string
	Terms      []string
	Analyzer   AnalyzerKey
	Filters    map[string]preparedFilter
	Facets     []FacetRequest
	Sort       SortKind
	Size       int
	Cursor     Cursor
	Highlights []TextField
	Digest     string
}

type preparedFilter struct {
	Values []string
	All    bool
}

func (catalog *Catalog) prepareQuery(query Query) (preparedQuery, error) {
	text := normalizeText(query.Text)
	if len(text) > catalog.definition.Limits.MaxQueryBytes {
		return preparedQuery{}, &Error{Kind: ErrorCapacity, Field: "text", Message: "exceeds the query byte budget"}
	}
	if _, exists := catalog.analyzers[query.Analyzer]; !exists {
		return preparedQuery{}, &Error{Kind: ErrorInvalidQuery, Field: "analyzer", Message: "is not declared"}
	}
	out := preparedQuery{
		Text: text, Terms: strings.Fields(strings.ToLower(text)), Analyzer: query.Analyzer,
		Filters: make(map[string]preparedFilter), Facets: append([]FacetRequest(nil), query.Facets...),
		Sort: query.Sort, Cursor: query.Page.Cursor,
	}
	if out.Sort == "" {
		out.Sort = SortRelevance
	}
	if out.Sort != SortRelevance && out.Sort != SortNewest {
		return preparedQuery{}, &Error{Kind: ErrorInvalidQuery, Field: "sort", Message: "is unsupported"}
	}
	out.Size = query.Page.Size
	if out.Size <= 0 {
		out.Size = catalog.definition.Limits.DefaultPageSize
	}
	if out.Size > catalog.definition.Limits.MaxPageSize {
		return preparedQuery{}, &Error{Kind: ErrorCapacity, Field: "page.size", Message: "exceeds the page budget"}
	}
	for _, filter := range query.Filters {
		definition, exists := catalog.filters[filter.name]
		if !exists {
			return preparedQuery{}, &Error{Kind: ErrorInvalidQuery, Field: "filters." + string(filter.name), Message: "is not declared"}
		}
		if _, exists := out.Filters[string(filter.name)]; exists {
			return preparedQuery{}, &Error{Kind: ErrorInvalidQuery, Field: "filters." + string(filter.name), Message: "is duplicated"}
		}
		values, err := normalizedStrings(filter.values, definition.MaxValues, definition.MaxBytes)
		if err != nil || len(values) == 0 {
			return preparedQuery{}, &Error{Kind: ErrorInvalidQuery, Field: "filters." + string(filter.name), Message: "is invalid"}
		}
		out.Filters[string(filter.name)] = preparedFilter{Values: values, All: filter.all}
	}
	if len(out.Facets) > catalog.definition.Limits.MaxFacets {
		return preparedQuery{}, &Error{Kind: ErrorCapacity, Field: "facets", Message: "exceeds the facet budget"}
	}
	facetNames := make(map[FilterName]struct{}, len(out.Facets))
	for index := range out.Facets {
		definition, exists := catalog.filters[out.Facets[index].Name]
		if !exists || !definition.Facetable {
			return preparedQuery{}, &Error{Kind: ErrorInvalidQuery, Field: "facets", Message: "contains a non-facetable field"}
		}
		if _, exists := facetNames[out.Facets[index].Name]; exists {
			return preparedQuery{}, &Error{Kind: ErrorInvalidQuery, Field: "facets", Message: "contains a duplicate field"}
		}
		facetNames[out.Facets[index].Name] = struct{}{}
		if out.Facets[index].Limit <= 0 {
			out.Facets[index].Limit = 10
		}
		if out.Facets[index].Limit > catalog.definition.Limits.MaxFacetBuckets {
			return preparedQuery{}, &Error{Kind: ErrorCapacity, Field: "facets", Message: "exceeds the bucket budget"}
		}
	}
	highlightFields := make(map[TextField]struct{}, len(query.Highlight.Fields))
	for _, field := range query.Highlight.Fields {
		if field != TextTitle && field != TextSummary && field != TextBody {
			return preparedQuery{}, &Error{Kind: ErrorInvalidQuery, Field: "highlight.fields", Message: "contains an unknown field"}
		}
		if _, exists := highlightFields[field]; exists {
			return preparedQuery{}, &Error{Kind: ErrorInvalidQuery, Field: "highlight.fields", Message: "contains a duplicate field"}
		}
		highlightFields[field] = struct{}{}
		out.Highlights = append(out.Highlights, field)
	}
	encoded, _ := json.Marshal(struct {
		Text     string
		Analyzer AnalyzerKey
		Filters  map[string]preparedFilter
		Facets   []FacetRequest
		Sort     SortKind
	}{out.Text, out.Analyzer, out.Filters, out.Facets, out.Sort})
	sum := sha256.Sum256(append([]byte(catalog.digest), encoded...))
	out.Digest = hex.EncodeToString(sum[:])
	return out, nil
}

func normalizeText(value string) string {
	value = norm.NFKC.String(value)
	var builder strings.Builder
	space := false
	for _, current := range value {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			if builder.Len() > 0 {
				space = true
			}
			continue
		}
		if space {
			builder.WriteByte(' ')
			space = false
		}
		builder.WriteRune(current)
	}
	return strings.TrimSpace(builder.String())
}
