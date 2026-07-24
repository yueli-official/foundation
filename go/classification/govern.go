package classification

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

type GovernOperation string

const (
	GovernSetStatus GovernOperation = "set_status"
	GovernReparent  GovernOperation = "reparent"
	GovernMerge     GovernOperation = "merge"
	GovernDelete    GovernOperation = "delete"
)

type GovernIdentityKind string

const (
	GovernCategory   GovernIdentityKind = "category"
	GovernFacet      GovernIdentityKind = "facet"
	GovernFacetValue GovernIdentityKind = "facet_value"
	GovernTag        GovernIdentityKind = "tag"
)

type ChildMove struct {
	ChildID  string
	ParentID string
}

type GovernCommand struct {
	Operation        GovernOperation
	Kind             GovernIdentityKind
	ID               string
	TargetID         string
	ParentID         string
	Status           Status
	ChildPlan        []ChildMove
	DeleteAllRelated bool
}

type GovernRequest struct {
	PolicyKey string
	Command   GovernCommand
}

type ImpactRequest struct {
	Kind GovernIdentityKind
	ID   string
}

type ReferenceImpact struct {
	Kind             GovernIdentityKind
	ID               string
	Exists           bool
	Status           Status
	DirectChildIDs   []string
	AssignmentCount  int64
	PrimaryCount     int64
	AliasCount       int64
	ReplacementCount int64
	HistoricalCount  int64
}

type GovernFactRequest struct {
	CatalogRevision uint64
	RequestToken    string
	Impacts         []ImpactRequest
}

type GovernFacts struct {
	CatalogRevision uint64
	RequestToken    string
	FreshnessToken  string
	Impacts         []ReferenceImpact
}

type GovernStepKind string

const (
	GovernChangeStatus            GovernStepKind = "change_status"
	GovernMoveChild               GovernStepKind = "move_child"
	GovernMigrateAssignments      GovernStepKind = "migrate_assignments"
	GovernMigratePrimary          GovernStepKind = "migrate_primary"
	GovernMigrateAliases          GovernStepKind = "migrate_aliases"
	GovernSetReplacement          GovernStepKind = "set_replacement"
	GovernClearPrimaryAssignments GovernStepKind = "clear_primary_assignments"
	GovernDeleteAssignments       GovernStepKind = "delete_assignments"
	GovernDeleteAliases           GovernStepKind = "delete_aliases"
	GovernDeleteReferences        GovernStepKind = "delete_references"
	GovernDeleteIdentity          GovernStepKind = "delete_identity"
)

type GovernStep struct {
	Kind          GovernStepKind
	IdentityKind  GovernIdentityKind
	SourceID      string
	TargetID      string
	ParentID      string
	Status        Status
	AffectedCount int64
}

type GovernancePlan struct {
	ExpectedCatalogRevision uint64
	ExpectedRequestToken    string
	ExpectedImpactToken     string
	Steps                   []GovernStep
}

type GovernResult struct {
	CatalogRevision uint64
	Outcome         Outcome
	Diagnostics     []Diagnostic
	Plan            GovernancePlan
}

type GovernPreparation struct {
	catalog     *Catalog
	factRequest GovernFactRequest
	command     GovernCommand
	diagnostics []Diagnostic
	deleteOrder []ImpactRequest
}

const (
	DiagnosticGovernChildPlanIncomplete        DiagnosticCode = "govern.child_plan_incomplete"
	DiagnosticGovernCycle                      DiagnosticCode = "govern.cycle"
	DiagnosticGovernDeleteConfirmationRequired DiagnosticCode = "govern.delete_confirmation_required"
	DiagnosticGovernIdentityUnknown            DiagnosticCode = "govern.identity_unknown"
	DiagnosticGovernOperationUnsupported       DiagnosticCode = "govern.operation_unsupported"
	DiagnosticGovernScopeMismatch              DiagnosticCode = "govern.scope_mismatch"
	DiagnosticGovernStatusTransitionInvalid    DiagnosticCode = "govern.status_transition_invalid"
)

func (c *Catalog) Govern(request GovernRequest) GovernPreparation {
	command := cloneGovernCommand(request.Command)
	command.ID = strings.TrimSpace(command.ID)
	command.TargetID = strings.TrimSpace(command.TargetID)
	command.ParentID = strings.TrimSpace(command.ParentID)
	diagnostics := make([]Diagnostic, 0)
	if _, exists := c.policyByKey[strings.TrimSpace(request.PolicyKey)]; !exists {
		diagnostics = append(diagnostics, Diagnostic{
			Code: DiagnosticPolicyUnknown, Path: []string{"policyKey"}, Reference: strings.TrimSpace(request.PolicyKey),
		})
	}
	if command.Kind != GovernTag && !c.governIdentityExists(command.Kind, command.ID) {
		diagnostics = append(diagnostics, Diagnostic{
			Code: DiagnosticGovernIdentityUnknown, Path: []string{"command", "id"}, Reference: command.ID,
		})
	}

	deleteOrder := make([]ImpactRequest, 0)
	impacts := make([]ImpactRequest, 0)
	switch command.Operation {
	case GovernSetStatus:
		if command.Kind == GovernTag {
			if command.Status != StatusActive && command.Status != StatusInactive {
				diagnostics = append(diagnostics, Diagnostic{
					Code: DiagnosticGovernStatusTransitionInvalid, Path: []string{"command", "status"}, Reference: command.ID,
				})
			}
			impacts = append(impacts, ImpactRequest{Kind: command.Kind, ID: command.ID})
		} else if !c.validStatusTransition(command.Kind, command.ID, command.Status) {
			diagnostics = append(diagnostics, Diagnostic{
				Code: DiagnosticGovernStatusTransitionInvalid, Path: []string{"command", "status"}, Reference: command.ID,
			})
		}
	case GovernReparent:
		diagnostics = append(diagnostics, c.validateReparent(command)...)
	case GovernMerge:
		diagnostics = append(diagnostics, c.validateMerge(command)...)
		if command.Kind != GovernFacet {
			impacts = append(impacts,
				ImpactRequest{Kind: command.Kind, ID: command.ID},
				ImpactRequest{Kind: command.Kind, ID: command.TargetID},
			)
		}
	case GovernDelete:
		deleteOrder = c.deleteImpactOrder(command.Kind, command.ID)
		if command.Kind == GovernTag {
			deleteOrder = []ImpactRequest{{Kind: command.Kind, ID: command.ID}}
		}
		impacts = append(impacts, deleteOrder...)
	default:
		diagnostics = append(diagnostics, Diagnostic{
			Code: DiagnosticGovernOperationUnsupported, Path: []string{"command", "operation"}, Reference: string(command.Operation),
		})
	}
	impacts = uniqueImpactRequests(impacts)
	token := governRequestToken(c.snapshot.Revision, strings.TrimSpace(request.PolicyKey), command, impacts)
	return GovernPreparation{
		catalog:     c,
		factRequest: GovernFactRequest{CatalogRevision: c.snapshot.Revision, RequestToken: token, Impacts: impacts},
		command:     command, diagnostics: diagnostics, deleteOrder: deleteOrder,
	}
}

func (p GovernPreparation) FactRequest() GovernFactRequest {
	request := p.factRequest
	request.Impacts = append([]ImpactRequest(nil), p.factRequest.Impacts...)
	return request
}

func (p GovernPreparation) Complete(facts GovernFacts) GovernResult {
	if facts.CatalogRevision != p.factRequest.CatalogRevision {
		return rejectedGovern(p.factRequest.CatalogRevision, DiagnosticFactsRevisionMismatch, []string{"facts", "catalogRevision"}, "")
	}
	if facts.RequestToken != p.factRequest.RequestToken {
		return rejectedGovern(p.factRequest.CatalogRevision, DiagnosticFactsTokenMismatch, []string{"facts", "requestToken"}, "")
	}
	if len(p.factRequest.Impacts) != 0 && strings.TrimSpace(facts.FreshnessToken) == "" {
		return rejectedGovern(p.factRequest.CatalogRevision, DiagnosticFactsFreshnessMissing, []string{"facts", "freshnessToken"}, "")
	}
	impactByKey := make(map[string]ReferenceImpact, len(facts.Impacts))
	for _, impact := range facts.Impacts {
		impact.DirectChildIDs = uniqueSortedStrings(impact.DirectChildIDs)
		impactByKey[impactKey(impact.Kind, impact.ID)] = impact
	}
	for _, request := range p.factRequest.Impacts {
		if _, exists := impactByKey[impactKey(request.Kind, request.ID)]; !exists {
			return rejectedGovern(p.factRequest.CatalogRevision, DiagnosticFactsIncomplete, []string{"facts", "impacts"}, request.ID)
		}
	}
	if len(p.diagnostics) != 0 {
		return GovernResult{
			CatalogRevision: p.factRequest.CatalogRevision, Outcome: OutcomeRejected,
			Diagnostics: append([]Diagnostic(nil), p.diagnostics...),
		}
	}
	if p.command.Kind == GovernTag {
		for _, request := range p.factRequest.Impacts {
			if !impactByKey[impactKey(request.Kind, request.ID)].Exists {
				return rejectedGovern(p.factRequest.CatalogRevision, DiagnosticGovernIdentityUnknown, []string{"command", "id"}, request.ID)
			}
		}
		if len(p.diagnostics) == 0 && !validTagGovernanceTransition(p.command, impactByKey) {
			return rejectedGovern(p.factRequest.CatalogRevision, DiagnosticGovernStatusTransitionInvalid, []string{"command", "status"}, p.command.ID)
		}
	}

	impactToken := ""
	if len(p.factRequest.Impacts) != 0 {
		impactToken = strings.TrimSpace(facts.FreshnessToken)
	}
	plan := GovernancePlan{
		ExpectedCatalogRevision: p.factRequest.CatalogRevision,
		ExpectedRequestToken:    p.factRequest.RequestToken,
		ExpectedImpactToken:     impactToken,
		Steps:                   make([]GovernStep, 0),
	}
	switch p.command.Operation {
	case GovernSetStatus:
		plan.Steps = append(plan.Steps, GovernStep{
			Kind: GovernChangeStatus, IdentityKind: p.command.Kind, SourceID: p.command.ID, Status: p.command.Status,
		})
	case GovernReparent:
		plan.Steps = append(plan.Steps, GovernStep{
			Kind: GovernMoveChild, IdentityKind: p.command.Kind, SourceID: p.command.ID, ParentID: p.command.ParentID,
		})
	case GovernMerge:
		source := impactByKey[impactKey(p.command.Kind, p.command.ID)]
		if !completeChildPlan(source.DirectChildIDs, p.command.ChildPlan) {
			return rejectedGovern(p.factRequest.CatalogRevision, DiagnosticGovernChildPlanIncomplete, []string{"command", "childPlan"}, p.command.ID)
		}
		if diagnostic := p.validateMergeChildPlan(); diagnostic != nil {
			return rejectedGovern(p.factRequest.CatalogRevision, diagnostic.Code, diagnostic.Path, diagnostic.Reference)
		}
		for _, child := range p.command.ChildPlan {
			plan.Steps = append(plan.Steps, GovernStep{
				Kind: GovernMoveChild, IdentityKind: p.command.Kind, SourceID: child.ChildID, ParentID: child.ParentID,
			})
		}
		if source.AssignmentCount > 0 {
			plan.Steps = append(plan.Steps, GovernStep{Kind: GovernMigrateAssignments, IdentityKind: p.command.Kind, SourceID: p.command.ID, TargetID: p.command.TargetID, AffectedCount: source.AssignmentCount})
		}
		if source.PrimaryCount > 0 {
			plan.Steps = append(plan.Steps, GovernStep{Kind: GovernMigratePrimary, IdentityKind: p.command.Kind, SourceID: p.command.ID, TargetID: p.command.TargetID, AffectedCount: source.PrimaryCount})
		}
		if source.AliasCount > 0 {
			plan.Steps = append(plan.Steps, GovernStep{Kind: GovernMigrateAliases, IdentityKind: p.command.Kind, SourceID: p.command.ID, TargetID: p.command.TargetID, AffectedCount: source.AliasCount})
		}
		plan.Steps = append(plan.Steps, GovernStep{Kind: GovernSetReplacement, IdentityKind: p.command.Kind, SourceID: p.command.ID, TargetID: p.command.TargetID})
	case GovernDelete:
		if !p.command.DeleteAllRelated && hasRelatedImpact(p.factRequest.Impacts, impactByKey) {
			return rejectedGovern(p.factRequest.CatalogRevision, DiagnosticGovernDeleteConfirmationRequired, []string{"command", "deleteAllRelated"}, p.command.ID)
		}
		for _, request := range p.deleteOrder {
			impact := impactByKey[impactKey(request.Kind, request.ID)]
			plan.Steps = append(plan.Steps, deleteSteps(impact)...)
		}
	}
	return GovernResult{
		CatalogRevision: p.factRequest.CatalogRevision, Outcome: OutcomePlanned,
		Diagnostics: []Diagnostic{}, Plan: plan,
	}
}

func (c *Catalog) governIdentityExists(kind GovernIdentityKind, id string) bool {
	switch kind {
	case GovernCategory:
		_, exists := c.categoryByID[id]
		return exists
	case GovernFacet:
		_, exists := c.facetByID[id]
		return exists
	case GovernFacetValue:
		_, exists := c.facetValueByID[id]
		return exists
	default:
		return false
	}
}

func (c *Catalog) validStatusTransition(kind GovernIdentityKind, id string, target Status) bool {
	if target != StatusActive && target != StatusInactive {
		return false
	}
	var current Status
	switch kind {
	case GovernCategory:
		category := c.categoryByID[id]
		current = category.Status
		if current == StatusDraft && target == StatusInactive {
			return false
		}
		if target == StatusActive && category.ParentID != "" && !c.categoryAvailability[category.ParentID].active {
			return false
		}
	case GovernFacet:
		current = c.facetByID[id].Status
		if current == StatusDraft && target == StatusInactive {
			return false
		}
	case GovernFacetValue:
		value := c.facetValueByID[id]
		current = value.Status
		if current == StatusDraft && target == StatusInactive {
			return false
		}
		if target == StatusActive {
			if !c.facetAvailability[value.FacetID].active {
				return false
			}
			if value.ParentID != "" && !c.facetValueAvailability[value.ParentID].active {
				return false
			}
		}
	default:
		return false
	}
	return current != StatusReplaced
}

func (c *Catalog) validateReparent(command GovernCommand) []Diagnostic {
	if command.Kind == GovernCategory {
		if source, exists := c.categoryByID[command.ID]; exists && source.Status == StatusReplaced {
			return []Diagnostic{{Code: DiagnosticGovernStatusTransitionInvalid, Path: []string{"command", "id"}, Reference: command.ID}}
		}
		if command.ParentID != "" {
			parent, exists := c.categoryByID[command.ParentID]
			if !exists {
				return []Diagnostic{{Code: DiagnosticGovernIdentityUnknown, Path: []string{"command", "parentId"}, Reference: command.ParentID}}
			}
			if parent.Status == StatusReplaced {
				return []Diagnostic{{Code: DiagnosticGovernStatusTransitionInvalid, Path: []string{"command", "parentId"}, Reference: command.ParentID}}
			}
			if containsString(c.categoryDescendants[command.ID], command.ParentID) {
				return []Diagnostic{{Code: DiagnosticGovernCycle, Path: []string{"command", "parentId"}, Reference: command.ParentID}}
			}
		}
		return nil
	}
	if command.Kind == GovernFacetValue {
		source, exists := c.facetValueByID[command.ID]
		if !exists {
			return nil
		}
		if source.Status == StatusReplaced {
			return []Diagnostic{{Code: DiagnosticGovernStatusTransitionInvalid, Path: []string{"command", "id"}, Reference: command.ID}}
		}
		if command.ParentID != "" {
			parent, parentExists := c.facetValueByID[command.ParentID]
			if !parentExists {
				return []Diagnostic{{Code: DiagnosticGovernIdentityUnknown, Path: []string{"command", "parentId"}, Reference: command.ParentID}}
			}
			if source.FacetID != parent.FacetID {
				return []Diagnostic{{Code: DiagnosticGovernScopeMismatch, Path: []string{"command", "parentId"}, Reference: command.ParentID}}
			}
			if parent.Status == StatusReplaced {
				return []Diagnostic{{Code: DiagnosticGovernStatusTransitionInvalid, Path: []string{"command", "parentId"}, Reference: command.ParentID}}
			}
			if containsString(c.facetValueDescendants[command.ID], command.ParentID) {
				return []Diagnostic{{Code: DiagnosticGovernCycle, Path: []string{"command", "parentId"}, Reference: command.ParentID}}
			}
		}
		return nil
	}
	return []Diagnostic{{Code: DiagnosticGovernOperationUnsupported, Path: []string{"command", "kind"}, Reference: string(command.Kind)}}
}

func (c *Catalog) validateMerge(command GovernCommand) []Diagnostic {
	if command.ID == command.TargetID {
		return []Diagnostic{{Code: DiagnosticGovernCycle, Path: []string{"command", "targetId"}, Reference: command.TargetID}}
	}
	if command.Kind == GovernTag {
		return nil
	}
	if !c.governIdentityExists(command.Kind, command.TargetID) {
		return []Diagnostic{{Code: DiagnosticGovernIdentityUnknown, Path: []string{"command", "targetId"}, Reference: command.TargetID}}
	}
	switch command.Kind {
	case GovernCategory:
		if c.categoryByID[command.ID].Status == StatusReplaced || !c.categoryAvailability[command.TargetID].active {
			return []Diagnostic{{Code: DiagnosticGovernStatusTransitionInvalid, Path: []string{"command", "targetId"}, Reference: command.TargetID}}
		}
		if containsString(c.categoryDescendants[command.ID], command.TargetID) {
			return []Diagnostic{{Code: DiagnosticGovernCycle, Path: []string{"command", "targetId"}, Reference: command.TargetID}}
		}
	case GovernFacetValue:
		source := c.facetValueByID[command.ID]
		target := c.facetValueByID[command.TargetID]
		if source.Status == StatusReplaced || !c.facetValueAvailability[target.ID].active {
			return []Diagnostic{{Code: DiagnosticGovernStatusTransitionInvalid, Path: []string{"command", "targetId"}, Reference: command.TargetID}}
		}
		if source.FacetID != target.FacetID {
			return []Diagnostic{{Code: DiagnosticGovernScopeMismatch, Path: []string{"command", "targetId"}, Reference: command.TargetID}}
		}
		if containsString(c.facetValueDescendants[command.ID], command.TargetID) {
			return []Diagnostic{{Code: DiagnosticGovernCycle, Path: []string{"command", "targetId"}, Reference: command.TargetID}}
		}
	case GovernFacet:
		return []Diagnostic{{Code: DiagnosticGovernOperationUnsupported, Path: []string{"command", "kind"}, Reference: string(command.Kind)}}
	default:
		return []Diagnostic{{Code: DiagnosticGovernOperationUnsupported, Path: []string{"command", "kind"}, Reference: string(command.Kind)}}
	}
	return nil
}

func validTagGovernanceTransition(command GovernCommand, impacts map[string]ReferenceImpact) bool {
	source := impacts[impactKey(GovernTag, command.ID)]
	switch command.Operation {
	case GovernSetStatus:
		return (command.Status == StatusActive || command.Status == StatusInactive) && source.Status != StatusReplaced
	case GovernMerge:
		target := impacts[impactKey(GovernTag, command.TargetID)]
		return source.Status != StatusReplaced && target.Status == StatusActive
	default:
		return true
	}
}

func (p GovernPreparation) validateMergeChildPlan() *Diagnostic {
	if len(p.command.ChildPlan) == 0 {
		return nil
	}
	parents := make(map[string]string)
	availabilityByID := make(map[string]availability)
	switch p.command.Kind {
	case GovernCategory:
		for _, category := range p.commandCatalogCategories() {
			parents[category.ID] = category.ParentID
			availabilityByID[category.ID] = p.commandCategoryAvailability(category.ID)
		}
	case GovernFacetValue:
		var facetID string
		for _, value := range p.commandCatalogFacetValues() {
			if value.ID == p.command.ID {
				facetID = value.FacetID
				break
			}
		}
		for _, value := range p.commandCatalogFacetValues() {
			if value.FacetID == facetID {
				parents[value.ID] = value.ParentID
				availabilityByID[value.ID] = p.commandFacetValueAvailability(value.ID)
			}
		}
	default:
		return nil
	}
	for _, move := range p.command.ChildPlan {
		if move.ParentID == p.command.ID || move.ParentID == move.ChildID {
			return &Diagnostic{Code: DiagnosticGovernCycle, Path: []string{"command", "childPlan"}, Reference: move.ChildID}
		}
		if move.ParentID != "" {
			if _, exists := parents[move.ParentID]; !exists {
				return &Diagnostic{Code: DiagnosticGovernScopeMismatch, Path: []string{"command", "childPlan"}, Reference: move.ParentID}
			}
			if !availabilityByID[move.ParentID].active {
				return &Diagnostic{Code: DiagnosticGovernStatusTransitionInvalid, Path: []string{"command", "childPlan"}, Reference: move.ParentID}
			}
		}
		parents[move.ChildID] = move.ParentID
	}
	parents[p.command.ID] = ""
	for id := range parents {
		seen := make(map[string]struct{})
		for current := id; current != ""; current = parents[current] {
			if _, exists := seen[current]; exists {
				return &Diagnostic{Code: DiagnosticGovernCycle, Path: []string{"command", "childPlan"}, Reference: id}
			}
			seen[current] = struct{}{}
		}
	}
	return nil
}

func (p GovernPreparation) commandCatalogCategories() []Category {
	return p.catalog.snapshot.Categories
}

func (p GovernPreparation) commandCategoryAvailability(id string) availability {
	return p.catalog.categoryAvailability[id]
}

func (p GovernPreparation) commandCatalogFacetValues() []FacetValue {
	return p.catalog.snapshot.FacetValues
}

func (p GovernPreparation) commandFacetValueAvailability(id string) availability {
	return p.catalog.facetValueAvailability[id]
}

func (c *Catalog) deleteImpactOrder(kind GovernIdentityKind, id string) []ImpactRequest {
	switch kind {
	case GovernCategory:
		ids := append([]string(nil), c.categoryDescendants[id]...)
		sort.Slice(ids, func(left, right int) bool {
			if c.categoryDepth[ids[left]] != c.categoryDepth[ids[right]] {
				return c.categoryDepth[ids[left]] > c.categoryDepth[ids[right]]
			}
			return ids[left] < ids[right]
		})
		return impactRequests(kind, ids)
	case GovernFacetValue:
		ids := append([]string(nil), c.facetValueDescendants[id]...)
		sort.Slice(ids, func(left, right int) bool {
			if c.facetValueDepth[ids[left]] != c.facetValueDepth[ids[right]] {
				return c.facetValueDepth[ids[left]] > c.facetValueDepth[ids[right]]
			}
			return ids[left] < ids[right]
		})
		return impactRequests(kind, ids)
	case GovernFacet:
		result := make([]ImpactRequest, 0)
		for _, value := range c.snapshot.FacetValues {
			if value.FacetID == id {
				result = append(result, ImpactRequest{Kind: GovernFacetValue, ID: value.ID})
			}
		}
		sort.Slice(result, func(left, right int) bool {
			if c.facetValueDepth[result[left].ID] != c.facetValueDepth[result[right].ID] {
				return c.facetValueDepth[result[left].ID] > c.facetValueDepth[result[right].ID]
			}
			return result[left].ID < result[right].ID
		})
		return append(result, ImpactRequest{Kind: GovernFacet, ID: id})
	default:
		return nil
	}
}

func deleteSteps(impact ReferenceImpact) []GovernStep {
	steps := make([]GovernStep, 0, 5)
	if impact.PrimaryCount > 0 {
		steps = append(steps, GovernStep{Kind: GovernClearPrimaryAssignments, IdentityKind: impact.Kind, SourceID: impact.ID, AffectedCount: impact.PrimaryCount})
	}
	if impact.AssignmentCount > 0 {
		steps = append(steps, GovernStep{Kind: GovernDeleteAssignments, IdentityKind: impact.Kind, SourceID: impact.ID, AffectedCount: impact.AssignmentCount})
	}
	if impact.AliasCount > 0 {
		steps = append(steps, GovernStep{Kind: GovernDeleteAliases, IdentityKind: impact.Kind, SourceID: impact.ID, AffectedCount: impact.AliasCount})
	}
	if impact.ReplacementCount+impact.HistoricalCount > 0 {
		steps = append(steps, GovernStep{Kind: GovernDeleteReferences, IdentityKind: impact.Kind, SourceID: impact.ID, AffectedCount: impact.ReplacementCount + impact.HistoricalCount})
	}
	steps = append(steps, GovernStep{Kind: GovernDeleteIdentity, IdentityKind: impact.Kind, SourceID: impact.ID})
	return steps
}

func hasRelatedImpact(requests []ImpactRequest, impacts map[string]ReferenceImpact) bool {
	if len(requests) > 1 {
		return true
	}
	for _, request := range requests {
		impact := impacts[impactKey(request.Kind, request.ID)]
		if len(impact.DirectChildIDs) != 0 || impact.AssignmentCount != 0 || impact.PrimaryCount != 0 ||
			impact.AliasCount != 0 || impact.ReplacementCount != 0 || impact.HistoricalCount != 0 {
			return true
		}
	}
	return false
}

func completeChildPlan(children []string, plan []ChildMove) bool {
	want := uniqueSortedStrings(children)
	got := make([]string, 0, len(plan))
	seen := make(map[string]struct{}, len(plan))
	for _, move := range plan {
		if move.ChildID == "" {
			return false
		}
		if _, exists := seen[move.ChildID]; exists {
			return false
		}
		seen[move.ChildID] = struct{}{}
		got = append(got, move.ChildID)
	}
	sort.Strings(got)
	if len(want) != len(got) {
		return false
	}
	for index := range want {
		if want[index] != got[index] {
			return false
		}
	}
	return true
}

func rejectedGovern(revision uint64, code DiagnosticCode, path []string, reference string) GovernResult {
	return GovernResult{
		CatalogRevision: revision, Outcome: OutcomeRejected,
		Diagnostics: []Diagnostic{{Code: code, Path: path, Reference: reference}},
	}
}

func cloneGovernCommand(command GovernCommand) GovernCommand {
	command.ChildPlan = append([]ChildMove(nil), command.ChildPlan...)
	return command
}

func impactRequests(kind GovernIdentityKind, ids []string) []ImpactRequest {
	result := make([]ImpactRequest, 0, len(ids))
	for _, id := range ids {
		result = append(result, ImpactRequest{Kind: kind, ID: id})
	}
	return result
}

func uniqueImpactRequests(values []ImpactRequest) []ImpactRequest {
	result := make([]ImpactRequest, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := impactKey(value.Kind, value.ID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func impactKey(kind GovernIdentityKind, id string) string { return string(kind) + "\x00" + id }

func containsString(values []string, value string) bool {
	index := sort.SearchStrings(values, value)
	return index < len(values) && values[index] == value
}

func governRequestToken(revision uint64, policyKey string, command GovernCommand, impacts []ImpactRequest) string {
	hash := sha256.New()
	writeTokenPart(hash, strconv.FormatUint(revision, 10), policyKey, string(command.Operation), string(command.Kind),
		command.ID, command.TargetID, command.ParentID, string(command.Status), strconv.FormatBool(command.DeleteAllRelated))
	for _, move := range command.ChildPlan {
		writeTokenPart(hash, move.ChildID, move.ParentID)
	}
	for _, impact := range impacts {
		writeTokenPart(hash, string(impact.Kind), impact.ID)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

type tokenWriter interface{ Write([]byte) (int, error) }

func writeTokenPart(writer tokenWriter, values ...string) {
	for _, value := range values {
		_, _ = writer.Write([]byte(value))
		_, _ = writer.Write([]byte{0})
	}
}
