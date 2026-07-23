package search

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

const DefinitionVersion uint64 = 1

var stableName = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

type AnalyzerDefinition struct {
	Key          AnalyzerKey
	QueryMode    QueryMode
	Required     []Capability
	ProbeText    string
	ProbeLexemes []string
}

type FilterDefinition struct {
	Name      FilterName
	Facetable bool
	MaxValues int
	MaxBytes  int
}

type Limits struct {
	MaxDocumentBytes  int
	MaxBatchDocuments int
	MaxBatchBytes     int
	DefaultPageSize   int
	MaxPageSize       int
	MaxFacets         int
	MaxFacetBuckets   int
	MaxHighlightBytes int
	MaxQueryBytes     int
}

type Definition struct {
	Consumer  string
	Version   uint64
	Analyzers []AnalyzerDefinition
	Filters   []FilterDefinition
	Limits    Limits
	CursorTTL time.Duration
}

type Catalog struct {
	definition Definition
	analyzers  map[AnalyzerKey]AnalyzerDefinition
	filters    map[FilterName]FilterDefinition
	digest     string
}

func Compile(definition Definition) (*Catalog, error) {
	if !stableName.MatchString(definition.Consumer) {
		return nil, definitionError("consumer", "must be a stable namespaced name")
	}
	if definition.Version == 0 {
		return nil, definitionError("version", "must be positive")
	}
	if len(definition.Analyzers) == 0 {
		return nil, definitionError("analyzers", "must not be empty")
	}
	definition.Limits = normalizeLimits(definition.Limits)
	if definition.CursorTTL <= 0 {
		definition.CursorTTL = 15 * time.Minute
	}
	catalog := &Catalog{
		definition: definition,
		analyzers:  make(map[AnalyzerKey]AnalyzerDefinition, len(definition.Analyzers)),
		filters:    make(map[FilterName]FilterDefinition, len(definition.Filters)),
	}
	for index, item := range definition.Analyzers {
		if !stableName.MatchString(string(item.Key)) {
			return nil, definitionError(fmt.Sprintf("analyzers[%d].key", index), "is invalid")
		}
		if item.QueryMode == "" {
			item.QueryMode = QueryWeb
			definition.Analyzers[index].QueryMode = QueryWeb
		}
		if item.QueryMode != QueryWeb && item.QueryMode != QueryPlain {
			return nil, definitionError(fmt.Sprintf("analyzers[%d].query_mode", index), "is unsupported")
		}
		if _, exists := catalog.analyzers[item.Key]; exists {
			return nil, definitionError(fmt.Sprintf("analyzers[%d].key", index), "is duplicated")
		}
		item.Required = slices.Clone(item.Required)
		item.ProbeLexemes = slices.Clone(item.ProbeLexemes)
		catalog.analyzers[item.Key] = item
	}
	for index, item := range definition.Filters {
		if !stableName.MatchString(string(item.Name)) {
			return nil, definitionError(fmt.Sprintf("filters[%d].name", index), "is invalid")
		}
		if item.MaxValues <= 0 {
			item.MaxValues = 20
			definition.Filters[index].MaxValues = item.MaxValues
		}
		if item.MaxBytes <= 0 {
			item.MaxBytes = 200
			definition.Filters[index].MaxBytes = item.MaxBytes
		}
		if _, exists := catalog.filters[item.Name]; exists {
			return nil, definitionError(fmt.Sprintf("filters[%d].name", index), "is duplicated")
		}
		catalog.filters[item.Name] = item
	}
	catalog.definition = definition
	encoded, _ := json.Marshal(definition)
	sum := sha256.Sum256(encoded)
	catalog.digest = hex.EncodeToString(sum[:])
	return catalog, nil
}

func MustCompile(definition Definition) *Catalog {
	value, err := Compile(definition)
	if err != nil {
		panic(err)
	}
	return value
}

func (catalog *Catalog) Definition() Definition { return catalog.definition }
func (catalog *Catalog) Digest() string         { return catalog.digest }

func normalizeLimits(value Limits) Limits {
	if value.MaxDocumentBytes <= 0 {
		value.MaxDocumentBytes = 2 << 20
	}
	if value.MaxBatchDocuments <= 0 {
		value.MaxBatchDocuments = 100
	}
	if value.MaxBatchBytes <= 0 {
		value.MaxBatchBytes = 8 << 20
	}
	if value.DefaultPageSize <= 0 {
		value.DefaultPageSize = 20
	}
	if value.MaxPageSize <= 0 {
		value.MaxPageSize = 100
	}
	if value.MaxFacets <= 0 {
		value.MaxFacets = 5
	}
	if value.MaxFacetBuckets <= 0 {
		value.MaxFacetBuckets = 50
	}
	if value.MaxHighlightBytes <= 0 {
		value.MaxHighlightBytes = 600
	}
	if value.MaxQueryBytes <= 0 {
		value.MaxQueryBytes = 500
	}
	return value
}

func definitionError(field, message string) error {
	return &Error{Kind: ErrorInvalidDefinition, Field: field, Message: message}
}

func normalizedStrings(values []string, maxValues, maxBytes int) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > maxBytes {
			return nil, fmt.Errorf("value exceeds %d bytes", maxBytes)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	if len(out) > maxValues {
		return nil, fmt.Errorf("contains more than %d values", maxValues)
	}
	return out, nil
}
