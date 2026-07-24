package classification

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	DiagnosticCategoryInactive           DiagnosticCode = "category.inactive"
	DiagnosticCategoryDepthExceeded      DiagnosticCode = "category.depth_exceeded"
	DiagnosticCategoryLeafRequired       DiagnosticCode = "category.leaf_required"
	DiagnosticCategoryTooFewAssignments  DiagnosticCode = "category.too_few_assignments"
	DiagnosticCategoryTooManyAssignments DiagnosticCode = "category.too_many_assignments"
	DiagnosticCategoryUnknown            DiagnosticCode = "category.unknown"
	DiagnosticFactsRevisionMismatch      DiagnosticCode = "facts.catalog_revision_mismatch"
	DiagnosticFactsTokenMismatch         DiagnosticCode = "facts.request_token_mismatch"
	DiagnosticFacetInactive              DiagnosticCode = "facet.inactive"
	DiagnosticFacetTooManyValues         DiagnosticCode = "facet.too_many_values"
	DiagnosticFacetTooFewValues          DiagnosticCode = "facet.too_few_values"
	DiagnosticFacetUnknown               DiagnosticCode = "facet.unknown"
	DiagnosticFacetValueInactive         DiagnosticCode = "facet_value.inactive"
	DiagnosticFacetValueUnknown          DiagnosticCode = "facet_value.unknown"
	DiagnosticFacetValueWrongFacet       DiagnosticCode = "facet_value.wrong_facet"
	DiagnosticFacetValueDepthExceeded    DiagnosticCode = "facet_value.depth_exceeded"
	DiagnosticFacetValueLeafRequired     DiagnosticCode = "facet_value.leaf_required"
	DiagnosticFactsIncomplete            DiagnosticCode = "facts.incomplete"
	DiagnosticFactsFreshnessMissing      DiagnosticCode = "facts.freshness_missing"
	DiagnosticPrimaryCategoryNotAssigned DiagnosticCode = "category.primary_not_assigned"
	DiagnosticPrimaryCategoryRequired    DiagnosticCode = "category.primary_required"
	DiagnosticTagRejected                DiagnosticCode = "tag.rejected"
	DiagnosticTagInactive                DiagnosticCode = "tag.inactive"
	DiagnosticTagTooFewAssignments       DiagnosticCode = "tag.too_few_assignments"
	DiagnosticTagTooManyAssignments      DiagnosticCode = "tag.too_many_assignments"
)

type ClassifyRequest struct {
	PolicyKey         string
	CategoryIDs       []string
	PrimaryCategoryID string
	Facets            []FacetSelection
	Tags              []string
}

type FacetSelection struct {
	FacetID  string
	ValueIDs []string
}

type CategoryAssignment struct {
	CategoryID string
}

type FacetValueAssignment struct {
	FacetID string
	ValueID string
}

type TagAssignment struct {
	TagID string
}

type ClassificationAssignments struct {
	Categories        []CategoryAssignment
	PrimaryCategoryID string
	Facets            []FacetValueAssignment
	Tags              []TagAssignment
}

type TagLookupRequest struct {
	LookupKey    string
	DisplayValue string
}

type TagMatchKind string

const (
	TagMatchCanonical   TagMatchKind = "canonical"
	TagMatchAlias       TagMatchKind = "alias"
	TagMatchReplacement TagMatchKind = "replacement"
	TagMatchInactive    TagMatchKind = "inactive"
	TagMatchNotFound    TagMatchKind = "not_found"
)

type TagMatch struct {
	LookupKey string
	Kind      TagMatchKind
	TagID     string
}

type TagProposalInput struct {
	LookupKey    string
	DisplayValue string
}

type TagCreationInput struct {
	LookupKey    string
	DisplayValue string
}

type ClassifyFactRequest struct {
	CatalogRevision uint64
	RequestToken    string
	TagLookups      []TagLookupRequest
}

type ClassifyFacts struct {
	CatalogRevision uint64
	RequestToken    string
	FreshnessToken  string
	TagMatches      []TagMatch
}

type ClassifyResult struct {
	CatalogRevision uint64
	Outcome         Outcome
	Diagnostics     []Diagnostic
	Assignments     ClassificationAssignments
	TagProposals    []TagProposalInput
	TagCreations    []TagCreationInput
}

type ClassifyPreparation struct {
	factRequest ClassifyFactRequest
	assignments ClassificationAssignments
	diagnostics []Diagnostic
	tagLookups  []TagLookupRequest
	policy      PolicyProfile
}

func (c *Catalog) Classify(request ClassifyRequest) ClassifyPreparation {
	categoryIDs := uniqueSortedStrings(request.CategoryIDs)
	assignments := ClassificationAssignments{
		Categories:        make([]CategoryAssignment, 0, len(categoryIDs)),
		PrimaryCategoryID: strings.TrimSpace(request.PrimaryCategoryID),
	}
	diagnostics := make([]Diagnostic, 0)
	for _, categoryID := range categoryIDs {
		if _, exists := c.categoryByID[categoryID]; !exists {
			diagnostics = append(diagnostics, Diagnostic{
				Code:      DiagnosticCategoryUnknown,
				Path:      []string{"categoryIds"},
				Reference: categoryID,
			})
			continue
		}
		if value := c.categoryAvailability[categoryID]; !value.active {
			diagnostics = append(diagnostics, Diagnostic{
				Code:      DiagnosticCategoryInactive,
				Path:      []string{"categoryIds"},
				Reference: categoryID,
				Params:    map[string]string{"reason": value.reason},
			})
			continue
		}
		assignments.Categories = append(assignments.Categories, CategoryAssignment{CategoryID: categoryID})
	}
	if assignments.PrimaryCategoryID != "" {
		assigned := false
		for _, assignment := range assignments.Categories {
			if assignment.CategoryID == assignments.PrimaryCategoryID {
				assigned = true
				break
			}
		}
		if !assigned {
			diagnostics = append(diagnostics, Diagnostic{
				Code:      DiagnosticPrimaryCategoryNotAssigned,
				Path:      []string{"primaryCategoryId"},
				Reference: assignments.PrimaryCategoryID,
			})
		}
	}
	selectedFacetValues := make(map[string]map[string]struct{})
	for _, selection := range request.Facets {
		facetID := strings.TrimSpace(selection.FacetID)
		if facetID == "" {
			continue
		}
		if _, exists := c.facetByID[facetID]; !exists {
			diagnostics = append(diagnostics, Diagnostic{
				Code:      DiagnosticFacetUnknown,
				Path:      []string{"facets"},
				Reference: facetID,
			})
			continue
		}
		if value := c.facetAvailability[facetID]; !value.active {
			diagnostics = append(diagnostics, Diagnostic{
				Code:      DiagnosticFacetInactive,
				Path:      []string{"facets", facetID},
				Reference: facetID,
				Params:    map[string]string{"reason": value.reason},
			})
			continue
		}
		if selectedFacetValues[facetID] == nil {
			selectedFacetValues[facetID] = make(map[string]struct{})
		}
		for _, valueID := range uniqueSortedStrings(selection.ValueIDs) {
			definition, exists := c.facetValueByID[valueID]
			if !exists {
				diagnostics = append(diagnostics, Diagnostic{
					Code:      DiagnosticFacetValueUnknown,
					Path:      []string{"facets", facetID, "valueIds"},
					Reference: valueID,
				})
				continue
			}
			if definition.FacetID != facetID {
				diagnostics = append(diagnostics, Diagnostic{
					Code:      DiagnosticFacetValueWrongFacet,
					Path:      []string{"facets", facetID, "valueIds"},
					Reference: valueID,
					Params:    map[string]string{"actualFacetId": definition.FacetID},
				})
				continue
			}
			if value := c.facetValueAvailability[valueID]; !value.active {
				diagnostics = append(diagnostics, Diagnostic{
					Code:      DiagnosticFacetValueInactive,
					Path:      []string{"facets", facetID, "valueIds"},
					Reference: valueID,
					Params:    map[string]string{"reason": value.reason},
				})
				continue
			}
			selectedFacetValues[facetID][valueID] = struct{}{}
		}
	}
	facetIDs := make([]string, 0, len(selectedFacetValues))
	for facetID := range selectedFacetValues {
		facetIDs = append(facetIDs, facetID)
	}
	sort.Strings(facetIDs)
	for _, facetID := range facetIDs {
		valueIDs := make([]string, 0, len(selectedFacetValues[facetID]))
		for valueID := range selectedFacetValues[facetID] {
			valueIDs = append(valueIDs, valueID)
		}
		sort.Strings(valueIDs)
		for _, valueID := range valueIDs {
			assignments.Facets = append(assignments.Facets, FacetValueAssignment{
				FacetID: facetID,
				ValueID: valueID,
			})
		}
	}
	policy := c.policyByKey[strings.TrimSpace(request.PolicyKey)]
	if policy.Key == "" {
		diagnostics = append(diagnostics, Diagnostic{
			Code:      DiagnosticPolicyUnknown,
			Path:      []string{"policyKey"},
			Reference: strings.TrimSpace(request.PolicyKey),
		})
	} else {
		categoryCount := len(assignments.Categories)
		if categoryCount < policy.Category.MinAssignments {
			diagnostics = append(diagnostics, Diagnostic{
				Code: DiagnosticCategoryTooFewAssignments, Path: []string{"categoryIds"},
				Params: map[string]string{"actual": strconv.Itoa(categoryCount), "min": strconv.Itoa(policy.Category.MinAssignments)},
			})
		}
		if policy.Category.MaxAssignments > 0 && categoryCount > policy.Category.MaxAssignments {
			diagnostics = append(diagnostics, Diagnostic{
				Code: DiagnosticCategoryTooManyAssignments, Path: []string{"categoryIds"},
				Params: map[string]string{"actual": strconv.Itoa(categoryCount), "max": strconv.Itoa(policy.Category.MaxAssignments)},
			})
		}
		for _, assignment := range assignments.Categories {
			if policy.Category.LeafOnly && c.categoryHasChildren[assignment.CategoryID] {
				diagnostics = append(diagnostics, Diagnostic{
					Code: DiagnosticCategoryLeafRequired, Path: []string{"categoryIds"}, Reference: assignment.CategoryID,
				})
			}
			if policy.Category.MaxDepth > 0 && c.categoryDepth[assignment.CategoryID] > policy.Category.MaxDepth {
				diagnostics = append(diagnostics, Diagnostic{
					Code: DiagnosticCategoryDepthExceeded, Path: []string{"categoryIds"}, Reference: assignment.CategoryID,
					Params: map[string]string{"actual": strconv.Itoa(c.categoryDepth[assignment.CategoryID]), "max": strconv.Itoa(policy.Category.MaxDepth)},
				})
			}
		}
		if policy.Category.RequirePrimary && assignments.PrimaryCategoryID == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code: DiagnosticPrimaryCategoryRequired,
				Path: []string{"primaryCategoryId"},
			})
		}
		for _, facetPolicy := range policy.Facets {
			count := len(selectedFacetValues[facetPolicy.FacetID])
			if count < facetPolicy.MinValues {
				diagnostics = append(diagnostics, Diagnostic{
					Code: DiagnosticFacetTooFewValues, Path: []string{"facets", facetPolicy.FacetID}, Reference: facetPolicy.FacetID,
					Params: map[string]string{"actual": strconv.Itoa(count), "min": strconv.Itoa(facetPolicy.MinValues)},
				})
			}
			if facetPolicy.MaxValues > 0 && count > facetPolicy.MaxValues {
				diagnostics = append(diagnostics, Diagnostic{
					Code:      DiagnosticFacetTooManyValues,
					Path:      []string{"facets", facetPolicy.FacetID},
					Reference: facetPolicy.FacetID,
					Params: map[string]string{
						"actual": strconv.Itoa(count),
						"max":    strconv.Itoa(facetPolicy.MaxValues),
					},
				})
			}
			valueIDs := sortedKeys(selectedFacetValues[facetPolicy.FacetID])
			for _, valueID := range valueIDs {
				if facetPolicy.LeafOnly && c.facetValueHasChildren[valueID] {
					diagnostics = append(diagnostics, Diagnostic{
						Code: DiagnosticFacetValueLeafRequired, Path: []string{"facets", facetPolicy.FacetID, "valueIds"}, Reference: valueID,
					})
				}
				if facetPolicy.MaxDepth > 0 && c.facetValueDepth[valueID] > facetPolicy.MaxDepth {
					diagnostics = append(diagnostics, Diagnostic{
						Code: DiagnosticFacetValueDepthExceeded, Path: []string{"facets", facetPolicy.FacetID, "valueIds"}, Reference: valueID,
						Params: map[string]string{"actual": strconv.Itoa(c.facetValueDepth[valueID]), "max": strconv.Itoa(facetPolicy.MaxDepth)},
					})
				}
			}
		}
	}
	tagLookups := normalizeTagLookups(request.Tags)
	token := classifyRequestToken(c.snapshot.Revision, strings.TrimSpace(request.PolicyKey), assignments, tagLookups)
	return ClassifyPreparation{
		factRequest: ClassifyFactRequest{
			CatalogRevision: c.snapshot.Revision,
			RequestToken:    token,
			TagLookups:      append([]TagLookupRequest(nil), tagLookups...),
		},
		assignments: assignments,
		diagnostics: diagnostics,
		tagLookups:  tagLookups,
		policy:      policy,
	}
}

func (p ClassifyPreparation) FactRequest() ClassifyFactRequest {
	result := p.factRequest
	result.TagLookups = append([]TagLookupRequest(nil), p.factRequest.TagLookups...)
	return result
}

func (p ClassifyPreparation) Complete(facts ClassifyFacts) ClassifyResult {
	if facts.CatalogRevision != p.factRequest.CatalogRevision {
		return ClassifyResult{
			CatalogRevision: p.factRequest.CatalogRevision,
			Outcome:         OutcomeRejected,
			Diagnostics: []Diagnostic{{
				Code: DiagnosticFactsRevisionMismatch,
				Path: []string{"facts", "catalogRevision"},
			}},
		}
	}
	if facts.RequestToken != p.factRequest.RequestToken {
		return ClassifyResult{
			CatalogRevision: p.factRequest.CatalogRevision,
			Outcome:         OutcomeRejected,
			Diagnostics: []Diagnostic{{
				Code: DiagnosticFactsTokenMismatch,
				Path: []string{"facts", "requestToken"},
			}},
		}
	}
	if len(p.tagLookups) != 0 && strings.TrimSpace(facts.FreshnessToken) == "" {
		return ClassifyResult{
			CatalogRevision: p.factRequest.CatalogRevision,
			Outcome:         OutcomeRejected,
			Diagnostics: []Diagnostic{{
				Code: DiagnosticFactsFreshnessMissing,
				Path: []string{"facts", "freshnessToken"},
			}},
		}
	}
	matches := make(map[string]TagMatch, len(facts.TagMatches))
	for _, match := range facts.TagMatches {
		matches[match.LookupKey] = match
	}
	for _, lookup := range p.tagLookups {
		if _, exists := matches[lookup.LookupKey]; !exists {
			return ClassifyResult{
				CatalogRevision: p.factRequest.CatalogRevision,
				Outcome:         OutcomeRejected,
				Diagnostics: []Diagnostic{{
					Code:      DiagnosticFactsIncomplete,
					Path:      []string{"facts", "tagMatches"},
					Reference: lookup.LookupKey,
				}},
			}
		}
	}
	if len(p.diagnostics) != 0 {
		return ClassifyResult{
			CatalogRevision: p.factRequest.CatalogRevision,
			Outcome:         OutcomeRejected,
			Diagnostics:     append([]Diagnostic(nil), p.diagnostics...),
		}
	}
	assignments := cloneAssignments(p.assignments)
	tagIDs := make(map[string]struct{})
	proposals := make([]TagProposalInput, 0)
	creations := make([]TagCreationInput, 0)
	diagnostics := make([]Diagnostic, 0)
	for _, lookup := range p.tagLookups {
		match := matches[lookup.LookupKey]
		switch match.Kind {
		case TagMatchCanonical, TagMatchAlias, TagMatchReplacement:
			if strings.TrimSpace(match.TagID) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Code:      DiagnosticFactsIncomplete,
					Path:      []string{"facts", "tagMatches"},
					Reference: lookup.LookupKey,
				})
				continue
			}
			tagIDs[strings.TrimSpace(match.TagID)] = struct{}{}
		case TagMatchNotFound:
			switch p.policy.Tags.Unknown {
			case UnknownTagPropose:
				proposals = append(proposals, TagProposalInput(lookup))
			case UnknownTagCreate:
				creations = append(creations, TagCreationInput(lookup))
			default:
				diagnostics = append(diagnostics, Diagnostic{
					Code:      DiagnosticTagRejected,
					Path:      []string{"tags"},
					Reference: lookup.LookupKey,
				})
			}
		case TagMatchInactive:
			diagnostics = append(diagnostics, Diagnostic{
				Code: DiagnosticTagInactive, Path: []string{"tags"}, Reference: lookup.LookupKey,
			})
		default:
			diagnostics = append(diagnostics, Diagnostic{
				Code:      DiagnosticFactsIncomplete,
				Path:      []string{"facts", "tagMatches"},
				Reference: lookup.LookupKey,
			})
		}
	}
	tagCount := len(tagIDs)
	if tagCount < p.policy.Tags.MinAssignments {
		diagnostics = append(diagnostics, Diagnostic{
			Code: DiagnosticTagTooFewAssignments, Path: []string{"tags"},
			Params: map[string]string{"actual": strconv.Itoa(tagCount), "min": strconv.Itoa(p.policy.Tags.MinAssignments)},
		})
	}
	if p.policy.Tags.MaxAssignments > 0 && tagCount > p.policy.Tags.MaxAssignments {
		diagnostics = append(diagnostics, Diagnostic{
			Code: DiagnosticTagTooManyAssignments, Path: []string{"tags"},
			Params: map[string]string{"actual": strconv.Itoa(tagCount), "max": strconv.Itoa(p.policy.Tags.MaxAssignments)},
		})
	}
	for _, tagID := range sortedKeys(tagIDs) {
		assignments.Tags = append(assignments.Tags, TagAssignment{TagID: tagID})
	}
	if len(diagnostics) != 0 {
		return ClassifyResult{
			CatalogRevision: p.factRequest.CatalogRevision,
			Outcome:         OutcomeRejected,
			Diagnostics:     diagnostics,
		}
	}
	return ClassifyResult{
		CatalogRevision: p.factRequest.CatalogRevision,
		Outcome:         OutcomeAccepted,
		Diagnostics:     []Diagnostic{},
		Assignments:     assignments,
		TagProposals:    proposals,
		TagCreations:    creations,
	}
}

func uniqueSortedStrings(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value != "" {
			unique[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func classifyRequestToken(
	revision uint64,
	policyKey string,
	assignments ClassificationAssignments,
	tagLookups []TagLookupRequest,
) string {
	hash := sha256.New()
	hash.Write([]byte(strconv.FormatUint(revision, 10)))
	hash.Write([]byte{0})
	hash.Write([]byte(policyKey))
	hash.Write([]byte{0})
	for _, assignment := range assignments.Categories {
		hash.Write([]byte(assignment.CategoryID))
		hash.Write([]byte{0})
	}
	hash.Write([]byte(assignments.PrimaryCategoryID))
	hash.Write([]byte{0})
	for _, assignment := range assignments.Facets {
		hash.Write([]byte(assignment.FacetID))
		hash.Write([]byte{0})
		hash.Write([]byte(assignment.ValueID))
		hash.Write([]byte{0})
	}
	for _, lookup := range tagLookups {
		hash.Write([]byte(lookup.LookupKey))
		hash.Write([]byte{0})
		hash.Write([]byte(lookup.DisplayValue))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func normalizeTagLookups(inputs []string) []TagLookupRequest {
	byKey := make(map[string]TagLookupRequest)
	for _, input := range inputs {
		display := strings.Join(strings.Fields(strings.TrimSpace(input)), " ")
		if display == "" {
			continue
		}
		lookupKey := cases.Fold().String(norm.NFKC.String(display))
		lookupKey = strings.Join(strings.Fields(lookupKey), " ")
		if _, exists := byKey[lookupKey]; !exists {
			byKey[lookupKey] = TagLookupRequest{
				LookupKey:    lookupKey,
				DisplayValue: display,
			}
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]TagLookupRequest, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result
}

func cloneAssignments(assignments ClassificationAssignments) ClassificationAssignments {
	cloned := assignments
	cloned.Categories = append([]CategoryAssignment(nil), assignments.Categories...)
	cloned.Facets = append([]FacetValueAssignment(nil), assignments.Facets...)
	cloned.Tags = append([]TagAssignment(nil), assignments.Tags...)
	return cloned
}
