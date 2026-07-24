package classification

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

type ReferenceKind string

const (
	ReferenceByID   ReferenceKind = "id"
	ReferenceBySlug ReferenceKind = "slug"
)

type Reference struct {
	Kind  ReferenceKind
	Value string
}

type FacetFilter struct {
	Facet  Reference
	Values []Reference
}

type CandidateProjectionMode string

const (
	CandidateProjectionNone      CandidateProjectionMode = ""
	CandidateProjectionAvailable CandidateProjectionMode = "available"
	CandidateProjectionActive    CandidateProjectionMode = "active"
)

const DiagnosticCandidateProjectionInvalid DiagnosticCode = "discover.candidate_projection_invalid"

type DiscoverRequest struct {
	PolicyKey           string
	Categories          []Reference
	Facets              []FacetFilter
	CandidateProjection CandidateProjectionMode
}

type FilterGroupKind string

const (
	FilterGroupCategory FilterGroupKind = "category"
	FilterGroupFacet    FilterGroupKind = "facet"
)

type FilterGroup struct {
	Kind     FilterGroupKind
	OwnerID  string
	ValueIDs []string
}

type FilterPlan struct {
	Groups []FilterGroup
}

// CandidateNode is a public, low-cardinality classification option. ParentID
// preserves the tree without exposing Catalog internals.
type CandidateNode struct {
	ID       string
	ParentID string
	Slug     string
	Name     string
	Count    int64
	Selected bool
}

type CandidateFacet struct {
	ID     string
	Slug   string
	Name   string
	Values []CandidateNode
}

type CandidateProjection struct {
	Categories []CandidateNode
	Facets     []CandidateFacet
}

type DiscoverFactRequest struct {
	CatalogRevision uint64
	RequestToken    string
	CountGroups     []CandidateCountGroupRequest
}

type DiscoverFacts struct {
	CatalogRevision uint64
	RequestToken    string
	FreshnessToken  string
	CountGroups     []CandidateCountGroup
}

type CandidateBucketRequest struct {
	ValueID     string
	MatchingIDs []string
}

type CandidateCountGroupRequest struct {
	Kind         FilterGroupKind
	OwnerID      string
	OtherFilters FilterPlan
	Candidates   []CandidateBucketRequest
}

type CandidateCount struct {
	ValueID string
	Count   int64
}

type CandidateCountGroup struct {
	Kind    FilterGroupKind
	OwnerID string
	Counts  []CandidateCount
}

type DiscoverResult struct {
	CatalogRevision uint64
	Outcome         Outcome
	Diagnostics     []Diagnostic
	FilterPlan      FilterPlan
	Candidates      CandidateProjection
}

type DiscoverPreparation struct {
	factRequest DiscoverFactRequest
	filterPlan  FilterPlan
	diagnostics []Diagnostic
	outcome     Outcome
	candidates  CandidateProjection
	countGroups []CandidateCountGroupRequest
}

func (c *Catalog) Discover(request DiscoverRequest) DiscoverPreparation {
	groups := make([]FilterGroup, 0, 1+len(request.Facets))
	diagnostics := make([]Diagnostic, 0)
	outcome := OutcomeAccepted
	projectionMode := request.CandidateProjection
	if projectionMode != CandidateProjectionNone && projectionMode != CandidateProjectionAvailable && projectionMode != CandidateProjectionActive {
		diagnostics = append(diagnostics, Diagnostic{
			Code: DiagnosticCandidateProjectionInvalid, Path: []string{"candidateProjection"}, Reference: string(projectionMode),
		})
		outcome = OutcomeRejected
		projectionMode = CandidateProjectionNone
	}
	if _, exists := c.policyByKey[strings.TrimSpace(request.PolicyKey)]; !exists {
		diagnostics = append(diagnostics, Diagnostic{
			Code:      DiagnosticPolicyUnknown,
			Path:      []string{"policyKey"},
			Reference: strings.TrimSpace(request.PolicyKey),
		})
		outcome = OutcomeRejected
	}
	categoryIDs := make(map[string]struct{})
	selectedCategoryIDs := make(map[string]struct{})
	for _, reference := range request.Categories {
		category, exists := c.resolveCategory(reference)
		if !exists {
			diagnostics = append(diagnostics, Diagnostic{
				Code:      DiagnosticCategoryUnknown,
				Path:      []string{"categories"},
				Reference: strings.TrimSpace(reference.Value),
			})
			continue
		}
		if !c.categoryAvailability[category.ID].active {
			diagnostics = append(diagnostics, Diagnostic{
				Code:      DiagnosticCategoryInactive,
				Path:      []string{"categories"},
				Reference: category.ID,
				Params:    map[string]string{"reason": c.categoryAvailability[category.ID].reason},
			})
			continue
		}
		for _, categoryID := range c.categoryDescendants[category.ID] {
			if c.categoryAvailability[categoryID].active {
				categoryIDs[categoryID] = struct{}{}
			}
		}
		selectedCategoryIDs[category.ID] = struct{}{}
	}
	if len(categoryIDs) != 0 {
		groups = append(groups, FilterGroup{
			Kind:     FilterGroupCategory,
			ValueIDs: sortedKeys(categoryIDs),
		})
	} else if len(request.Categories) != 0 {
		outcome = nonExecutableUnlessRejected(outcome)
	}

	facetValues := make(map[string]map[string]struct{})
	selectedFacetValues := make(map[string]map[string]struct{})
	requestedFacetGroups := make(map[string]bool)
	for _, filter := range request.Facets {
		facet, exists := c.resolveFacet(filter.Facet)
		if !exists {
			diagnostics = append(diagnostics, Diagnostic{
				Code:      DiagnosticFacetUnknown,
				Path:      []string{"facets"},
				Reference: strings.TrimSpace(filter.Facet.Value),
			})
			if len(filter.Values) != 0 {
				outcome = nonExecutableUnlessRejected(outcome)
			}
			continue
		}
		if !c.facetAvailability[facet.ID].active {
			diagnostics = append(diagnostics, Diagnostic{
				Code:      DiagnosticFacetInactive,
				Path:      []string{"facets", facet.ID},
				Reference: facet.ID,
				Params:    map[string]string{"reason": c.facetAvailability[facet.ID].reason},
			})
			if len(filter.Values) != 0 {
				outcome = nonExecutableUnlessRejected(outcome)
			}
			continue
		}
		if facetValues[facet.ID] == nil {
			facetValues[facet.ID] = make(map[string]struct{})
		}
		if selectedFacetValues[facet.ID] == nil {
			selectedFacetValues[facet.ID] = make(map[string]struct{})
		}
		if len(filter.Values) != 0 {
			requestedFacetGroups[facet.ID] = true
		}
		for _, reference := range filter.Values {
			value, valueExists := c.resolveFacetValue(facet.ID, reference)
			if !valueExists {
				diagnostics = append(diagnostics, Diagnostic{
					Code:      DiagnosticFacetValueUnknown,
					Path:      []string{"facets", facet.ID, "values"},
					Reference: strings.TrimSpace(reference.Value),
				})
				continue
			}
			if !c.facetValueAvailability[value.ID].active {
				diagnostics = append(diagnostics, Diagnostic{
					Code:      DiagnosticFacetValueInactive,
					Path:      []string{"facets", facet.ID, "values"},
					Reference: value.ID,
					Params:    map[string]string{"reason": c.facetValueAvailability[value.ID].reason},
				})
				continue
			}
			for _, valueID := range c.facetValueDescendants[value.ID] {
				if c.facetValueAvailability[valueID].active {
					facetValues[facet.ID][valueID] = struct{}{}
				}
			}
			selectedFacetValues[facet.ID][value.ID] = struct{}{}
		}
	}
	facetIDs := make([]string, 0, len(facetValues))
	for facetID := range facetValues {
		if len(facetValues[facetID]) != 0 {
			facetIDs = append(facetIDs, facetID)
		} else if requestedFacetGroups[facetID] {
			outcome = nonExecutableUnlessRejected(outcome)
		}
	}
	sort.Strings(facetIDs)
	for _, facetID := range facetIDs {
		groups = append(groups, FilterGroup{
			Kind:     FilterGroupFacet,
			OwnerID:  facetID,
			ValueIDs: sortedKeys(facetValues[facetID]),
		})
	}

	plan := FilterPlan{Groups: groups}
	candidates := CandidateProjection{}
	countGroups := []CandidateCountGroupRequest{}
	if projectionMode != CandidateProjectionNone {
		candidates = c.candidateProjection()
		markSelectedCandidates(&candidates, selectedCategoryIDs, selectedFacetValues)
		if projectionMode == CandidateProjectionAvailable {
			countGroups = c.candidateCountRequests(candidates, plan)
		}
	}
	token := discoverRequestToken(c.snapshot.Revision, strings.TrimSpace(request.PolicyKey), projectionMode, plan, countGroups)
	return DiscoverPreparation{
		factRequest: DiscoverFactRequest{
			CatalogRevision: c.snapshot.Revision,
			RequestToken:    token,
			CountGroups:     cloneCandidateCountGroupRequests(countGroups),
		},
		filterPlan:  plan,
		diagnostics: diagnostics,
		outcome:     outcome,
		candidates:  candidates,
		countGroups: countGroups,
	}
}

func nonExecutableUnlessRejected(outcome Outcome) Outcome {
	if outcome == OutcomeRejected {
		return outcome
	}
	return OutcomeNonExecutable
}

func (p DiscoverPreparation) FactRequest() DiscoverFactRequest {
	request := p.factRequest
	request.CountGroups = cloneCandidateCountGroupRequests(p.factRequest.CountGroups)
	return request
}

func (p DiscoverPreparation) Complete(facts DiscoverFacts) DiscoverResult {
	if facts.CatalogRevision != p.factRequest.CatalogRevision {
		return DiscoverResult{
			CatalogRevision: p.factRequest.CatalogRevision,
			Outcome:         OutcomeRejected,
			Diagnostics: []Diagnostic{{
				Code: DiagnosticFactsRevisionMismatch,
				Path: []string{"facts", "catalogRevision"},
			}},
		}
	}
	if facts.RequestToken != p.factRequest.RequestToken {
		return DiscoverResult{
			CatalogRevision: p.factRequest.CatalogRevision,
			Outcome:         OutcomeRejected,
			Diagnostics: []Diagnostic{{
				Code: DiagnosticFactsTokenMismatch,
				Path: []string{"facts", "requestToken"},
			}},
		}
	}
	if len(p.countGroups) != 0 && strings.TrimSpace(facts.FreshnessToken) == "" {
		return DiscoverResult{
			CatalogRevision: p.factRequest.CatalogRevision,
			Outcome:         OutcomeRejected,
			Diagnostics: []Diagnostic{{
				Code: DiagnosticFactsFreshnessMissing, Path: []string{"facts", "freshnessToken"},
			}},
		}
	}
	candidates := cloneCandidateProjection(p.candidates)
	if len(p.countGroups) != 0 {
		groups := make(map[string]CandidateCountGroup, len(facts.CountGroups))
		for _, group := range facts.CountGroups {
			groups[filterGroupKey(group.Kind, group.OwnerID)] = group
		}
		for _, request := range p.countGroups {
			group, exists := groups[filterGroupKey(request.Kind, request.OwnerID)]
			if !exists {
				return incompleteDiscoverCounts(p.factRequest.CatalogRevision, request.OwnerID)
			}
			counts := make(map[string]int64, len(group.Counts))
			for _, count := range group.Counts {
				counts[count.ValueID] = count.Count
			}
			for _, candidate := range request.Candidates {
				if _, exists := counts[candidate.ValueID]; !exists {
					return incompleteDiscoverCounts(p.factRequest.CatalogRevision, candidate.ValueID)
				}
			}
			applyCandidateCounts(&candidates, request.Kind, request.OwnerID, counts)
		}
		filterZeroCandidates(&candidates)
	}
	return DiscoverResult{
		CatalogRevision: p.factRequest.CatalogRevision,
		Outcome:         p.outcome,
		Diagnostics:     append([]Diagnostic(nil), p.diagnostics...),
		FilterPlan:      p.filterPlan,
		Candidates:      candidates,
	}
}

func (c *Catalog) candidateCountRequests(projection CandidateProjection, plan FilterPlan) []CandidateCountGroupRequest {
	groups := make([]CandidateCountGroupRequest, 0, 1+len(projection.Facets))
	if len(projection.Categories) != 0 {
		request := CandidateCountGroupRequest{
			Kind: FilterGroupCategory, OtherFilters: excludeFilterGroup(plan, FilterGroupCategory, ""),
		}
		for _, candidate := range projection.Categories {
			request.Candidates = append(request.Candidates, CandidateBucketRequest{
				ValueID: candidate.ID, MatchingIDs: activeDescendants(c.categoryDescendants[candidate.ID], c.categoryAvailability),
			})
		}
		groups = append(groups, request)
	}
	for _, facet := range projection.Facets {
		request := CandidateCountGroupRequest{
			Kind: FilterGroupFacet, OwnerID: facet.ID,
			OtherFilters: excludeFilterGroup(plan, FilterGroupFacet, facet.ID),
		}
		for _, candidate := range facet.Values {
			request.Candidates = append(request.Candidates, CandidateBucketRequest{
				ValueID: candidate.ID, MatchingIDs: activeDescendants(c.facetValueDescendants[candidate.ID], c.facetValueAvailability),
			})
		}
		groups = append(groups, request)
	}
	return groups
}

func activeDescendants(ids []string, availabilityByID map[string]availability) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if availabilityByID[id].active {
			result = append(result, id)
		}
	}
	return result
}

func excludeFilterGroup(plan FilterPlan, kind FilterGroupKind, ownerID string) FilterPlan {
	result := FilterPlan{Groups: make([]FilterGroup, 0, len(plan.Groups))}
	for _, group := range plan.Groups {
		if group.Kind == kind && group.OwnerID == ownerID {
			continue
		}
		result.Groups = append(result.Groups, FilterGroup{
			Kind: group.Kind, OwnerID: group.OwnerID, ValueIDs: append([]string(nil), group.ValueIDs...),
		})
	}
	return result
}

func markSelectedCandidates(projection *CandidateProjection, categories map[string]struct{}, facets map[string]map[string]struct{}) {
	for index := range projection.Categories {
		_, projection.Categories[index].Selected = categories[projection.Categories[index].ID]
	}
	for facetIndex := range projection.Facets {
		selected := facets[projection.Facets[facetIndex].ID]
		for valueIndex := range projection.Facets[facetIndex].Values {
			_, projection.Facets[facetIndex].Values[valueIndex].Selected = selected[projection.Facets[facetIndex].Values[valueIndex].ID]
		}
	}
}

func applyCandidateCounts(projection *CandidateProjection, kind FilterGroupKind, ownerID string, counts map[string]int64) {
	if kind == FilterGroupCategory {
		for index := range projection.Categories {
			projection.Categories[index].Count = counts[projection.Categories[index].ID]
		}
		return
	}
	for facetIndex := range projection.Facets {
		if projection.Facets[facetIndex].ID != ownerID {
			continue
		}
		for valueIndex := range projection.Facets[facetIndex].Values {
			projection.Facets[facetIndex].Values[valueIndex].Count = counts[projection.Facets[facetIndex].Values[valueIndex].ID]
		}
	}
}

func filterZeroCandidates(projection *CandidateProjection) {
	projection.Categories = retainCandidatePaths(projection.Categories)
	filteredFacets := make([]CandidateFacet, 0, len(projection.Facets))
	for _, facet := range projection.Facets {
		facet.Values = retainCandidatePaths(facet.Values)
		if len(facet.Values) != 0 {
			filteredFacets = append(filteredFacets, facet)
		}
	}
	projection.Facets = filteredFacets
}

func retainCandidatePaths(values []CandidateNode) []CandidateNode {
	byID := make(map[string]CandidateNode, len(values))
	keep := make(map[string]struct{})
	for _, value := range values {
		byID[value.ID] = value
		if value.Count > 0 || value.Selected {
			keep[value.ID] = struct{}{}
		}
	}
	for id := range keep {
		parentID := byID[id].ParentID
		for parentID != "" {
			keep[parentID] = struct{}{}
			parentID = byID[parentID].ParentID
		}
	}
	result := make([]CandidateNode, 0, len(keep))
	for _, value := range values {
		if _, exists := keep[value.ID]; exists {
			result = append(result, value)
		}
	}
	return result
}

func incompleteDiscoverCounts(revision uint64, reference string) DiscoverResult {
	return DiscoverResult{
		CatalogRevision: revision, Outcome: OutcomeRejected,
		Diagnostics: []Diagnostic{{Code: DiagnosticFactsIncomplete, Path: []string{"facts", "countGroups"}, Reference: reference}},
	}
}

func filterGroupKey(kind FilterGroupKind, ownerID string) string {
	return string(kind) + "\x00" + ownerID
}

func cloneCandidateCountGroupRequests(values []CandidateCountGroupRequest) []CandidateCountGroupRequest {
	result := make([]CandidateCountGroupRequest, len(values))
	for index, value := range values {
		result[index] = value
		result[index].OtherFilters = cloneFilterPlan(value.OtherFilters)
		result[index].Candidates = make([]CandidateBucketRequest, len(value.Candidates))
		for candidateIndex, candidate := range value.Candidates {
			result[index].Candidates[candidateIndex] = candidate
			result[index].Candidates[candidateIndex].MatchingIDs = append([]string(nil), candidate.MatchingIDs...)
		}
	}
	return result
}

func cloneFilterPlan(plan FilterPlan) FilterPlan {
	result := FilterPlan{Groups: make([]FilterGroup, len(plan.Groups))}
	for index, group := range plan.Groups {
		result.Groups[index] = group
		result.Groups[index].ValueIDs = append([]string(nil), group.ValueIDs...)
	}
	return result
}

func (c *Catalog) candidateProjection() CandidateProjection {
	categoryDefinitions := make([]candidateDefinition, 0, len(c.snapshot.Categories))
	for _, category := range c.snapshot.Categories {
		if c.categoryAvailability[category.ID].active {
			categoryDefinitions = append(categoryDefinitions, candidateDefinition{
				ID: category.ID, ParentID: category.ParentID, Slug: category.Slug, Name: category.Name,
				EditorialPosition: category.EditorialPosition,
			})
		}
	}
	projection := CandidateProjection{Categories: orderCandidateTree(categoryDefinitions)}

	facets := append([]Facet(nil), c.snapshot.Facets...)
	sort.Slice(facets, func(left, right int) bool {
		return candidateLess(
			facets[left].EditorialPosition, facets[left].Name, facets[left].ID,
			facets[right].EditorialPosition, facets[right].Name, facets[right].ID,
		)
	})
	for _, facet := range facets {
		if !c.facetAvailability[facet.ID].active {
			continue
		}
		definitions := make([]candidateDefinition, 0)
		for _, value := range c.snapshot.FacetValues {
			if value.FacetID == facet.ID && c.facetValueAvailability[value.ID].active {
				definitions = append(definitions, candidateDefinition{
					ID: value.ID, ParentID: value.ParentID, Slug: value.Slug, Name: value.Name,
					EditorialPosition: value.EditorialPosition,
				})
			}
		}
		projection.Facets = append(projection.Facets, CandidateFacet{
			ID: facet.ID, Slug: facet.Slug, Name: facet.Name, Values: orderCandidateTree(definitions),
		})
	}
	return projection
}

type candidateDefinition struct {
	ID                string
	ParentID          string
	Slug              string
	Name              string
	EditorialPosition *int
}

func orderCandidateTree(definitions []candidateDefinition) []CandidateNode {
	byID := make(map[string]candidateDefinition, len(definitions))
	children := make(map[string][]candidateDefinition)
	for _, definition := range definitions {
		byID[definition.ID] = definition
	}
	for _, definition := range definitions {
		parentID := definition.ParentID
		if _, exists := byID[parentID]; !exists {
			parentID = ""
		}
		children[parentID] = append(children[parentID], definition)
	}
	for parentID := range children {
		sort.Slice(children[parentID], func(left, right int) bool {
			leftValue := children[parentID][left]
			rightValue := children[parentID][right]
			return candidateLess(
				leftValue.EditorialPosition, leftValue.Name, leftValue.ID,
				rightValue.EditorialPosition, rightValue.Name, rightValue.ID,
			)
		})
	}
	result := make([]CandidateNode, 0, len(definitions))
	var appendChildren func(string)
	appendChildren = func(parentID string) {
		for _, definition := range children[parentID] {
			result = append(result, CandidateNode{
				ID: definition.ID, ParentID: definition.ParentID, Slug: definition.Slug, Name: definition.Name,
			})
			appendChildren(definition.ID)
		}
	}
	appendChildren("")
	return result
}

func candidateLess(leftPosition *int, leftName, leftID string, rightPosition *int, rightName, rightID string) bool {
	if leftPosition != nil || rightPosition != nil {
		if leftPosition == nil {
			return false
		}
		if rightPosition == nil {
			return true
		}
		if *leftPosition != *rightPosition {
			return *leftPosition < *rightPosition
		}
	}
	if leftName != rightName {
		return leftName < rightName
	}
	return leftID < rightID
}

func cloneCandidateProjection(projection CandidateProjection) CandidateProjection {
	cloned := CandidateProjection{
		Categories: append([]CandidateNode(nil), projection.Categories...),
		Facets:     make([]CandidateFacet, len(projection.Facets)),
	}
	for index, facet := range projection.Facets {
		cloned.Facets[index] = facet
		cloned.Facets[index].Values = append([]CandidateNode(nil), facet.Values...)
	}
	return cloned
}

func (c *Catalog) resolveCategory(reference Reference) (Category, bool) {
	value := strings.TrimSpace(reference.Value)
	switch reference.Kind {
	case ReferenceByID:
		category, exists := c.categoryByID[value]
		return category, exists
	case ReferenceBySlug:
		category, exists := c.categoryBySlug[value]
		return category, exists
	default:
		return Category{}, false
	}
}

func (c *Catalog) resolveFacet(reference Reference) (Facet, bool) {
	value := strings.TrimSpace(reference.Value)
	switch reference.Kind {
	case ReferenceByID:
		facet, exists := c.facetByID[value]
		return facet, exists
	case ReferenceBySlug:
		facet, exists := c.facetBySlug[value]
		return facet, exists
	default:
		return Facet{}, false
	}
}

func (c *Catalog) resolveFacetValue(facetID string, reference Reference) (FacetValue, bool) {
	value := strings.TrimSpace(reference.Value)
	switch reference.Kind {
	case ReferenceByID:
		definition, exists := c.facetValueByID[value]
		return definition, exists && definition.FacetID == facetID
	case ReferenceBySlug:
		definition, exists := c.facetValueByFacetSlug[facetValueSlugKey(facetID, value)]
		return definition, exists
	default:
		return FacetValue{}, false
	}
}

func discoverRequestToken(revision uint64, policyKey string, projection CandidateProjectionMode, plan FilterPlan, countGroups []CandidateCountGroupRequest) string {
	hash := sha256.New()
	hash.Write([]byte(strconv.FormatUint(revision, 10)))
	hash.Write([]byte{0})
	hash.Write([]byte(policyKey))
	hash.Write([]byte{0})
	hash.Write([]byte(projection))
	hash.Write([]byte{0})
	for _, group := range plan.Groups {
		hash.Write([]byte(group.Kind))
		hash.Write([]byte{0})
		hash.Write([]byte(group.OwnerID))
		hash.Write([]byte{0})
		for _, valueID := range group.ValueIDs {
			hash.Write([]byte(valueID))
			hash.Write([]byte{0})
		}
	}
	for _, group := range countGroups {
		hash.Write([]byte(group.Kind))
		hash.Write([]byte{0})
		hash.Write([]byte(group.OwnerID))
		hash.Write([]byte{0})
		for _, candidate := range group.Candidates {
			hash.Write([]byte(candidate.ValueID))
			hash.Write([]byte{0})
			for _, id := range candidate.MatchingIDs {
				hash.Write([]byte(id))
				hash.Write([]byte{0})
			}
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}
