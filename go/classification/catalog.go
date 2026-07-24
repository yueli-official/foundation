// Package classification owns reusable deterministic rules for categories,
// facets and tags. Consumers own persistence, object queries and transactions.
package classification

import "sort"

type Status string

const (
	StatusDraft    Status = "draft"
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
	StatusReplaced Status = "replaced"
)

type Outcome string

const (
	OutcomeAccepted      Outcome = "accepted"
	OutcomeRejected      Outcome = "rejected"
	OutcomeNonExecutable Outcome = "non_executable"
	OutcomePlanned       Outcome = "planned"
)

type CandidateSort string

const (
	CandidateSortEditorial CandidateSort = "editorial"
	CandidateSortCountDesc CandidateSort = "count_desc"
	CandidateSortNameAsc   CandidateSort = "name_asc"
)

type UnknownTagAdmission string

const (
	UnknownTagCreate  UnknownTagAdmission = "create"
	UnknownTagPropose UnknownTagAdmission = "propose"
	UnknownTagReject  UnknownTagAdmission = "reject"
)

type Category struct {
	ID                string
	ParentID          string
	Slug              string
	Name              string
	Status            Status
	EditorialPosition *int
	ReplacementID     string
}

type Facet struct {
	ID                string
	Slug              string
	Name              string
	Status            Status
	EditorialPosition *int
	ReplacementID     string
}

type FacetValue struct {
	ID                string
	FacetID           string
	ParentID          string
	Slug              string
	Name              string
	Status            Status
	EditorialPosition *int
	ReplacementID     string
}

type CategoryPolicy struct {
	MinAssignments int
	MaxAssignments int
	RequirePrimary bool
	LeafOnly       bool
	MaxDepth       int
}

type FacetAssignmentPolicy struct {
	FacetID   string
	MinValues int
	MaxValues int
	LeafOnly  bool
	MaxDepth  int
}

type TagAdmissionPolicy struct {
	Unknown        UnknownTagAdmission
	MinAssignments int
	MaxAssignments int
}

type DiscoveryPolicy struct {
	DefaultSort CandidateSort
}

type PolicyProfile struct {
	Key            string
	SchemaVersion  uint16
	PolicyRevision uint64
	Category       CategoryPolicy
	Facets         []FacetAssignmentPolicy
	Tags           TagAdmissionPolicy
	Discovery      DiscoveryPolicy
}

type Snapshot struct {
	CatalogID   string
	Revision    uint64
	Categories  []Category
	Facets      []Facet
	FacetValues []FacetValue
	Policies    []PolicyProfile
}

type DiagnosticCode string

const (
	DiagnosticCatalogInvalid               DiagnosticCode = "catalog.invalid"
	DiagnosticCategoryDuplicateID          DiagnosticCode = "category.duplicate_id"
	DiagnosticCategoryDuplicateSlug        DiagnosticCode = "category.duplicate_slug"
	DiagnosticCategoryParentUnknown        DiagnosticCode = "category.parent_unknown"
	DiagnosticCategoryReplacementInvalid   DiagnosticCode = "category.replacement_invalid"
	DiagnosticCategoryStatusInvalid        DiagnosticCode = "category.status_invalid"
	DiagnosticCategoryCycle                DiagnosticCode = "category.cycle"
	DiagnosticFacetDuplicateID             DiagnosticCode = "facet.duplicate_id"
	DiagnosticFacetDuplicateSlug           DiagnosticCode = "facet.duplicate_slug"
	DiagnosticFacetReplacementInvalid      DiagnosticCode = "facet.replacement_invalid"
	DiagnosticFacetStatusInvalid           DiagnosticCode = "facet.status_invalid"
	DiagnosticFacetValueCycle              DiagnosticCode = "facet_value.cycle"
	DiagnosticFacetValueDuplicateID        DiagnosticCode = "facet_value.duplicate_id"
	DiagnosticFacetValueDuplicateSlug      DiagnosticCode = "facet_value.duplicate_slug"
	DiagnosticFacetValueFacetUnknown       DiagnosticCode = "facet_value.facet_unknown"
	DiagnosticFacetValueParentScope        DiagnosticCode = "facet_value.parent_scope"
	DiagnosticFacetValueReplacementInvalid DiagnosticCode = "facet_value.replacement_invalid"
	DiagnosticFacetValueStatusInvalid      DiagnosticCode = "facet_value.status_invalid"
	DiagnosticPolicyDuplicateKey           DiagnosticCode = "policy.duplicate_key"
	DiagnosticPolicyFacetUnknown           DiagnosticCode = "policy.facet_unknown"
	DiagnosticPolicyRangeInvalid           DiagnosticCode = "policy.range_invalid"
	DiagnosticPolicyVersionInvalid         DiagnosticCode = "policy.version_invalid"
	DiagnosticPolicyUnknown                DiagnosticCode = "policy.unknown"
)

type Diagnostic struct {
	Code      DiagnosticCode
	Path      []string
	Reference string
	Params    map[string]string
}

type CompileResult struct {
	CatalogRevision uint64
	Outcome         Outcome
	Diagnostics     []Diagnostic
	Catalog         *Catalog
}

type Catalog struct {
	snapshot               Snapshot
	categoryByID           map[string]Category
	categoryBySlug         map[string]Category
	categoryAvailability   map[string]availability
	categoryDescendants    map[string][]string
	categoryDepth          map[string]int
	categoryHasChildren    map[string]bool
	facetByID              map[string]Facet
	facetBySlug            map[string]Facet
	facetAvailability      map[string]availability
	facetValueByID         map[string]FacetValue
	facetValueByFacetSlug  map[string]FacetValue
	facetValueAvailability map[string]availability
	facetValueDescendants  map[string][]string
	facetValueDepth        map[string]int
	facetValueHasChildren  map[string]bool
	policyByKey            map[string]PolicyProfile
}

type availability struct {
	active bool
	reason string
}

func Compile(snapshot Snapshot) CompileResult {
	compiled := cloneSnapshot(snapshot)
	if diagnostic := validateCatalogIdentities(compiled); diagnostic != nil {
		return CompileResult{
			CatalogRevision: snapshot.Revision,
			Outcome:         OutcomeRejected,
			Diagnostics:     []Diagnostic{*diagnostic},
		}
	}
	if diagnostic := findCategoryCycle(compiled.Categories); diagnostic != nil {
		return CompileResult{
			CatalogRevision: snapshot.Revision,
			Outcome:         OutcomeRejected,
			Diagnostics:     []Diagnostic{*diagnostic},
		}
	}
	if diagnostic := validateFacetValueParents(compiled.FacetValues); diagnostic != nil {
		return CompileResult{
			CatalogRevision: snapshot.Revision,
			Outcome:         OutcomeRejected,
			Diagnostics:     []Diagnostic{*diagnostic},
		}
	}
	if diagnostic := findFacetValueCycle(compiled.FacetValues); diagnostic != nil {
		return CompileResult{
			CatalogRevision: snapshot.Revision,
			Outcome:         OutcomeRejected,
			Diagnostics:     []Diagnostic{*diagnostic},
		}
	}
	categoryByID := make(map[string]Category, len(compiled.Categories))
	categoryBySlug := make(map[string]Category, len(compiled.Categories))
	for _, category := range compiled.Categories {
		categoryByID[category.ID] = category
		categoryBySlug[category.Slug] = category
	}
	categoryAvailability := compileCategoryAvailability(compiled.Categories, categoryByID)
	categoryDescendants := compileCategoryDescendants(compiled.Categories)
	categoryDepth, categoryHasChildren := compileCategoryShape(compiled.Categories, categoryByID)
	facetByID := make(map[string]Facet, len(compiled.Facets))
	facetBySlug := make(map[string]Facet, len(compiled.Facets))
	facetAvailability := make(map[string]availability, len(compiled.Facets))
	for _, facet := range compiled.Facets {
		facetByID[facet.ID] = facet
		facetBySlug[facet.Slug] = facet
		if facet.Status == StatusActive {
			facetAvailability[facet.ID] = availability{active: true}
		} else {
			facetAvailability[facet.ID] = availability{reason: "self_" + string(facet.Status)}
		}
	}
	facetValueByID := make(map[string]FacetValue, len(compiled.FacetValues))
	facetValueByFacetSlug := make(map[string]FacetValue, len(compiled.FacetValues))
	for _, value := range compiled.FacetValues {
		facetValueByID[value.ID] = value
		facetValueByFacetSlug[facetValueSlugKey(value.FacetID, value.Slug)] = value
	}
	facetValueAvailability := compileFacetValueAvailability(compiled.FacetValues, facetAvailability, facetValueByID)
	facetValueDescendants := compileFacetValueDescendants(compiled.FacetValues)
	facetValueDepth, facetValueHasChildren := compileFacetValueShape(compiled.FacetValues, facetValueByID)
	if diagnostic := validatePolicyFacetReferences(compiled.Policies, facetByID); diagnostic != nil {
		return CompileResult{
			CatalogRevision: snapshot.Revision,
			Outcome:         OutcomeRejected,
			Diagnostics:     []Diagnostic{*diagnostic},
		}
	}
	if diagnostic := validatePolicyRanges(compiled.Policies); diagnostic != nil {
		return CompileResult{
			CatalogRevision: snapshot.Revision,
			Outcome:         OutcomeRejected,
			Diagnostics:     []Diagnostic{*diagnostic},
		}
	}
	policyByKey := make(map[string]PolicyProfile, len(compiled.Policies))
	for _, policy := range compiled.Policies {
		policyByKey[policy.Key] = policy
	}
	return CompileResult{
		CatalogRevision: snapshot.Revision,
		Outcome:         OutcomeAccepted,
		Diagnostics:     []Diagnostic{},
		Catalog: &Catalog{
			snapshot:               compiled,
			categoryByID:           categoryByID,
			categoryBySlug:         categoryBySlug,
			categoryAvailability:   categoryAvailability,
			categoryDescendants:    categoryDescendants,
			categoryDepth:          categoryDepth,
			categoryHasChildren:    categoryHasChildren,
			facetByID:              facetByID,
			facetBySlug:            facetBySlug,
			facetAvailability:      facetAvailability,
			facetValueByID:         facetValueByID,
			facetValueByFacetSlug:  facetValueByFacetSlug,
			facetValueAvailability: facetValueAvailability,
			facetValueDescendants:  facetValueDescendants,
			facetValueDepth:        facetValueDepth,
			facetValueHasChildren:  facetValueHasChildren,
			policyByKey:            policyByKey,
		},
	}
}

func validateCatalogIdentities(snapshot Snapshot) *Diagnostic {
	if snapshot.CatalogID == "" || snapshot.Revision == 0 {
		return &Diagnostic{Code: DiagnosticCatalogInvalid, Path: []string{"catalog"}}
	}
	categoryByID := make(map[string]Category, len(snapshot.Categories))
	categorySlug := make(map[string]struct{}, len(snapshot.Categories))
	for _, category := range snapshot.Categories {
		if category.ID == "" {
			return &Diagnostic{Code: DiagnosticCategoryDuplicateID, Path: []string{"categories", "id"}}
		}
		if _, exists := categoryByID[category.ID]; exists {
			return &Diagnostic{Code: DiagnosticCategoryDuplicateID, Path: []string{"categories", category.ID}, Reference: category.ID}
		}
		categoryByID[category.ID] = category
		if category.Slug == "" {
			return &Diagnostic{Code: DiagnosticCategoryDuplicateSlug, Path: []string{"categories", category.ID, "slug"}, Reference: category.Slug}
		}
		if _, exists := categorySlug[category.Slug]; exists {
			return &Diagnostic{Code: DiagnosticCategoryDuplicateSlug, Path: []string{"categories", category.ID, "slug"}, Reference: category.Slug}
		}
		categorySlug[category.Slug] = struct{}{}
		if !validCatalogStatus(category.Status) {
			return &Diagnostic{Code: DiagnosticCategoryStatusInvalid, Path: []string{"categories", category.ID, "status"}, Reference: category.ID}
		}
	}
	for _, category := range snapshot.Categories {
		if category.ParentID != "" {
			if _, exists := categoryByID[category.ParentID]; !exists {
				return &Diagnostic{Code: DiagnosticCategoryParentUnknown, Path: []string{"categories", category.ID, "parentId"}, Reference: category.ParentID}
			}
		}
		if !validCategoryReplacement(category, categoryByID) {
			return &Diagnostic{Code: DiagnosticCategoryReplacementInvalid, Path: []string{"categories", category.ID, "replacementId"}, Reference: category.ReplacementID}
		}
	}

	facetByID := make(map[string]Facet, len(snapshot.Facets))
	facetSlug := make(map[string]struct{}, len(snapshot.Facets))
	for _, facet := range snapshot.Facets {
		if facet.ID == "" {
			return &Diagnostic{Code: DiagnosticFacetDuplicateID, Path: []string{"facets", "id"}}
		}
		if _, exists := facetByID[facet.ID]; exists {
			return &Diagnostic{Code: DiagnosticFacetDuplicateID, Path: []string{"facets", facet.ID}, Reference: facet.ID}
		}
		facetByID[facet.ID] = facet
		if facet.Slug == "" {
			return &Diagnostic{Code: DiagnosticFacetDuplicateSlug, Path: []string{"facets", facet.ID, "slug"}, Reference: facet.Slug}
		}
		if _, exists := facetSlug[facet.Slug]; exists {
			return &Diagnostic{Code: DiagnosticFacetDuplicateSlug, Path: []string{"facets", facet.ID, "slug"}, Reference: facet.Slug}
		}
		facetSlug[facet.Slug] = struct{}{}
		if !validCatalogStatus(facet.Status) {
			return &Diagnostic{Code: DiagnosticFacetStatusInvalid, Path: []string{"facets", facet.ID, "status"}, Reference: facet.ID}
		}
	}
	for _, facet := range snapshot.Facets {
		if !validFacetReplacement(facet, facetByID) {
			return &Diagnostic{Code: DiagnosticFacetReplacementInvalid, Path: []string{"facets", facet.ID, "replacementId"}, Reference: facet.ReplacementID}
		}
	}

	valueByID := make(map[string]FacetValue, len(snapshot.FacetValues))
	valueSlugs := make(map[string]struct{}, len(snapshot.FacetValues))
	for _, value := range snapshot.FacetValues {
		if value.ID == "" {
			return &Diagnostic{Code: DiagnosticFacetValueDuplicateID, Path: []string{"facetValues", "id"}}
		}
		if _, exists := valueByID[value.ID]; exists {
			return &Diagnostic{Code: DiagnosticFacetValueDuplicateID, Path: []string{"facetValues", value.ID}, Reference: value.ID}
		}
		valueByID[value.ID] = value
		if _, exists := facetByID[value.FacetID]; !exists {
			return &Diagnostic{Code: DiagnosticFacetValueFacetUnknown, Path: []string{"facetValues", value.ID, "facetId"}, Reference: value.FacetID}
		}
		key := facetValueSlugKey(value.FacetID, value.Slug)
		if value.Slug == "" {
			return &Diagnostic{Code: DiagnosticFacetValueDuplicateSlug, Path: []string{"facetValues", value.ID, "slug"}, Reference: value.Slug}
		}
		if _, exists := valueSlugs[key]; exists {
			return &Diagnostic{Code: DiagnosticFacetValueDuplicateSlug, Path: []string{"facetValues", value.ID, "slug"}, Reference: value.Slug}
		}
		valueSlugs[key] = struct{}{}
		if !validCatalogStatus(value.Status) {
			return &Diagnostic{Code: DiagnosticFacetValueStatusInvalid, Path: []string{"facetValues", value.ID, "status"}, Reference: value.ID}
		}
	}
	for _, value := range snapshot.FacetValues {
		if !validFacetValueReplacement(value, valueByID) {
			return &Diagnostic{Code: DiagnosticFacetValueReplacementInvalid, Path: []string{"facetValues", value.ID, "replacementId"}, Reference: value.ReplacementID}
		}
	}

	policyKeys := make(map[string]struct{}, len(snapshot.Policies))
	for _, policy := range snapshot.Policies {
		if policy.Key == "" {
			return &Diagnostic{Code: DiagnosticPolicyDuplicateKey, Path: []string{"policies", "key"}}
		}
		if _, exists := policyKeys[policy.Key]; exists {
			return &Diagnostic{Code: DiagnosticPolicyDuplicateKey, Path: []string{"policies", policy.Key}, Reference: policy.Key}
		}
		policyKeys[policy.Key] = struct{}{}
		if policy.SchemaVersion != 1 || policy.PolicyRevision == 0 {
			return &Diagnostic{Code: DiagnosticPolicyVersionInvalid, Path: []string{"policies", policy.Key, "schemaVersion"}, Reference: policy.Key}
		}
	}
	return nil
}

func validCatalogStatus(status Status) bool {
	return status == StatusDraft || status == StatusActive || status == StatusInactive || status == StatusReplaced
}

func validCategoryReplacement(value Category, byID map[string]Category) bool {
	if value.Status != StatusReplaced {
		return value.ReplacementID == ""
	}
	target, exists := byID[value.ReplacementID]
	return exists && target.ID != value.ID && target.Status != StatusReplaced
}

func validFacetReplacement(value Facet, byID map[string]Facet) bool {
	if value.Status != StatusReplaced {
		return value.ReplacementID == ""
	}
	target, exists := byID[value.ReplacementID]
	return exists && target.ID != value.ID && target.Status != StatusReplaced
}

func validFacetValueReplacement(value FacetValue, byID map[string]FacetValue) bool {
	if value.Status != StatusReplaced {
		return value.ReplacementID == ""
	}
	target, exists := byID[value.ReplacementID]
	return exists && target.ID != value.ID && target.FacetID == value.FacetID && target.Status != StatusReplaced
}

func validatePolicyRanges(policies []PolicyProfile) *Diagnostic {
	for _, policy := range policies {
		if !validRange(policy.Category.MinAssignments, policy.Category.MaxAssignments) || policy.Category.MaxDepth < 0 {
			return &Diagnostic{
				Code: DiagnosticPolicyRangeInvalid, Path: []string{"policies", policy.Key, "category"},
			}
		}
		for _, facetPolicy := range policy.Facets {
			if !validRange(facetPolicy.MinValues, facetPolicy.MaxValues) || facetPolicy.MaxDepth < 0 {
				return &Diagnostic{
					Code: DiagnosticPolicyRangeInvalid, Path: []string{"policies", policy.Key, "facets", facetPolicy.FacetID},
					Reference: facetPolicy.FacetID,
				}
			}
		}
		if !validRange(policy.Tags.MinAssignments, policy.Tags.MaxAssignments) {
			return &Diagnostic{
				Code: DiagnosticPolicyRangeInvalid, Path: []string{"policies", policy.Key, "tags"},
			}
		}
	}
	return nil
}

func validRange(minimum, maximum int) bool {
	return minimum >= 0 && maximum >= 0 && (maximum == 0 || minimum <= maximum)
}

func compileCategoryShape(categories []Category, byID map[string]Category) (map[string]int, map[string]bool) {
	depth := make(map[string]int, len(categories))
	hasChildren := make(map[string]bool, len(categories))
	var calculate func(Category) int
	calculate = func(category Category) int {
		if value := depth[category.ID]; value != 0 {
			return value
		}
		value := 1
		if parent, exists := byID[category.ParentID]; exists {
			value = calculate(parent) + 1
		}
		depth[category.ID] = value
		return value
	}
	for _, category := range categories {
		calculate(category)
		if category.ParentID != "" {
			hasChildren[category.ParentID] = true
		}
	}
	return depth, hasChildren
}

func compileFacetValueShape(values []FacetValue, byID map[string]FacetValue) (map[string]int, map[string]bool) {
	depth := make(map[string]int, len(values))
	hasChildren := make(map[string]bool, len(values))
	var calculate func(FacetValue) int
	calculate = func(value FacetValue) int {
		if result := depth[value.ID]; result != 0 {
			return result
		}
		result := 1
		if parent, exists := byID[value.ParentID]; exists {
			result = calculate(parent) + 1
		}
		depth[value.ID] = result
		return result
	}
	for _, value := range values {
		calculate(value)
		if value.ParentID != "" {
			hasChildren[value.ParentID] = true
		}
	}
	return depth, hasChildren
}

func validatePolicyFacetReferences(policies []PolicyProfile, facets map[string]Facet) *Diagnostic {
	for _, policy := range policies {
		for _, facetPolicy := range policy.Facets {
			if _, exists := facets[facetPolicy.FacetID]; !exists {
				return &Diagnostic{
					Code:      DiagnosticPolicyFacetUnknown,
					Path:      []string{"policies", policy.Key, "facets"},
					Reference: facetPolicy.FacetID,
				}
			}
		}
	}
	return nil
}

func compileCategoryDescendants(categories []Category) map[string][]string {
	children := make(map[string][]string)
	for _, category := range categories {
		if category.ParentID != "" {
			children[category.ParentID] = append(children[category.ParentID], category.ID)
		}
	}
	result := make(map[string][]string, len(categories))
	var collect func(string, map[string]struct{})
	collect = func(id string, values map[string]struct{}) {
		values[id] = struct{}{}
		for _, childID := range children[id] {
			collect(childID, values)
		}
	}
	for _, category := range categories {
		values := make(map[string]struct{})
		collect(category.ID, values)
		result[category.ID] = sortedKeys(values)
	}
	return result
}

func compileFacetValueDescendants(values []FacetValue) map[string][]string {
	children := make(map[string][]string)
	for _, value := range values {
		if value.ParentID != "" {
			children[value.ParentID] = append(children[value.ParentID], value.ID)
		}
	}
	result := make(map[string][]string, len(values))
	var collect func(string, map[string]struct{})
	collect = func(id string, collected map[string]struct{}) {
		collected[id] = struct{}{}
		for _, childID := range children[id] {
			collect(childID, collected)
		}
	}
	for _, value := range values {
		collected := make(map[string]struct{})
		collect(value.ID, collected)
		result[value.ID] = sortedKeys(collected)
	}
	return result
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func facetValueSlugKey(facetID, slug string) string {
	return facetID + "\x00" + slug
}

func findFacetValueCycle(values []FacetValue) *Diagnostic {
	byID := make(map[string]FacetValue, len(values))
	for _, value := range values {
		byID[value.ID] = value
	}
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(values))
	var visit func(FacetValue) *Diagnostic
	visit = func(value FacetValue) *Diagnostic {
		switch state[value.ID] {
		case visiting:
			return &Diagnostic{
				Code:      DiagnosticFacetValueCycle,
				Path:      []string{"facetValues", value.ID, "parentId"},
				Reference: value.ID,
			}
		case visited:
			return nil
		}
		state[value.ID] = visiting
		if parent, exists := byID[value.ParentID]; exists {
			if diagnostic := visit(parent); diagnostic != nil {
				return diagnostic
			}
		}
		state[value.ID] = visited
		return nil
	}
	for _, value := range values {
		if diagnostic := visit(value); diagnostic != nil {
			return diagnostic
		}
	}
	return nil
}

func validateFacetValueParents(values []FacetValue) *Diagnostic {
	byID := make(map[string]FacetValue, len(values))
	for _, value := range values {
		byID[value.ID] = value
	}
	for _, value := range values {
		if value.ParentID == "" {
			continue
		}
		parent, exists := byID[value.ParentID]
		if !exists || parent.FacetID != value.FacetID {
			reason := "unknown_parent"
			if exists {
				reason = "different_facet"
			}
			return &Diagnostic{
				Code:      DiagnosticFacetValueParentScope,
				Path:      []string{"facetValues", value.ID, "parentId"},
				Reference: value.ID,
				Params:    map[string]string{"reason": reason},
			}
		}
	}
	return nil
}

func compileFacetValueAvailability(
	values []FacetValue,
	facetAvailability map[string]availability,
	byID map[string]FacetValue,
) map[string]availability {
	result := make(map[string]availability, len(values))
	var evaluate func(FacetValue) availability
	evaluate = func(value FacetValue) availability {
		if compiled, exists := result[value.ID]; exists {
			return compiled
		}
		if !facetAvailability[value.FacetID].active {
			compiled := availability{reason: "facet_inactive"}
			result[value.ID] = compiled
			return compiled
		}
		if value.Status != StatusActive {
			compiled := availability{reason: "self_" + string(value.Status)}
			result[value.ID] = compiled
			return compiled
		}
		if parent, exists := byID[value.ParentID]; exists {
			if !evaluate(parent).active {
				compiled := availability{reason: "ancestor_inactive"}
				result[value.ID] = compiled
				return compiled
			}
		}
		compiled := availability{active: true}
		result[value.ID] = compiled
		return compiled
	}
	for _, value := range values {
		evaluate(value)
	}
	return result
}

func compileCategoryAvailability(categories []Category, byID map[string]Category) map[string]availability {
	result := make(map[string]availability, len(categories))
	var evaluate func(Category) availability
	evaluate = func(category Category) availability {
		if value, exists := result[category.ID]; exists {
			return value
		}
		if category.Status != StatusActive {
			value := availability{reason: "self_" + string(category.Status)}
			result[category.ID] = value
			return value
		}
		if parent, exists := byID[category.ParentID]; exists {
			if !evaluate(parent).active {
				value := availability{reason: "ancestor_inactive"}
				result[category.ID] = value
				return value
			}
		}
		value := availability{active: true}
		result[category.ID] = value
		return value
	}
	for _, category := range categories {
		evaluate(category)
	}
	return result
}

func findCategoryCycle(categories []Category) *Diagnostic {
	byID := make(map[string]Category, len(categories))
	for _, category := range categories {
		byID[category.ID] = category
	}
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(categories))
	var visit func(Category) *Diagnostic
	visit = func(category Category) *Diagnostic {
		switch state[category.ID] {
		case visiting:
			return &Diagnostic{
				Code:      DiagnosticCategoryCycle,
				Path:      []string{"categories", category.ID, "parentId"},
				Reference: category.ID,
			}
		case visited:
			return nil
		}
		state[category.ID] = visiting
		if parent, exists := byID[category.ParentID]; exists {
			if diagnostic := visit(parent); diagnostic != nil {
				return diagnostic
			}
		}
		state[category.ID] = visited
		return nil
	}
	for _, category := range categories {
		if diagnostic := visit(category); diagnostic != nil {
			return diagnostic
		}
	}
	return nil
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	cloned := snapshot
	cloned.Categories = append([]Category(nil), snapshot.Categories...)
	cloned.Facets = append([]Facet(nil), snapshot.Facets...)
	cloned.FacetValues = append([]FacetValue(nil), snapshot.FacetValues...)
	cloned.Policies = make([]PolicyProfile, len(snapshot.Policies))
	for index, policy := range snapshot.Policies {
		cloned.Policies[index] = policy
		cloned.Policies[index].Facets = append([]FacetAssignmentPolicy(nil), policy.Facets...)
	}
	return cloned
}
