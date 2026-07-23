package authorization

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryOptions configures the reference in-memory Adapter.
type MemoryOptions struct {
	RootScopeID       ScopeID
	ProtectedSubjects []SubjectRef
	Clock             func() time.Time
	TokenGenerator    func() (string, error)
	Constraints       map[ConstraintKey]ConstraintEvaluator
	Predicates        map[PredicateKey]PredicateEvaluator
}

// Memory is a concurrency-safe reference Adapter. It is useful for
// conformance tests and ephemeral consumers; it is not durable storage.
type Memory struct {
	mu               sync.RWMutex
	catalog          *Catalog
	clock            func() time.Time
	tokenGenerator   func() (string, error)
	nextID           uint64
	scopes           map[ScopeID]Scope
	grants           map[GrantID]Grant
	groups           map[GroupID]Group
	groupMembers     map[GroupID]map[string]SubjectRef
	applications     map[ApplicationID]Application
	invitations      map[InvitationID]Invitation
	invitationTokens map[[32]byte]InvitationID
	constraints      map[ConstraintKey]ConstraintEvaluator
	predicates       map[PredicateKey]PredicateEvaluator
	inbox            map[string]ReconcileResult
	activePolicy     uint64
	nextPolicy       uint64
	policies         map[uint64]*memoryPolicy
	audit            []AuditEvent
	decisionAudit    []DecisionAuditEvent
	casbin           *casbinProjection
}

var (
	_ Authorizer            = (*Memory)(nil)
	_ QueryPlanner          = (*Memory)(nil)
	_ AccessReader          = (*Memory)(nil)
	_ ScopeManager          = (*Memory)(nil)
	_ ResourceScopeRegistry = (*Memory)(nil)
	_ ScopeReader           = (*Memory)(nil)
	_ RoleManager           = (*Memory)(nil)
	_ RoleReader            = (*Memory)(nil)
	_ GrantManager          = (*Memory)(nil)
	_ GrantReader           = (*Memory)(nil)
	_ GroupManager          = (*Memory)(nil)
	_ GroupReader           = (*Memory)(nil)
	_ WorkflowManager       = (*Memory)(nil)
	_ WorkflowReader        = (*Memory)(nil)
	_ Reconciler            = (*Memory)(nil)
	_ PolicyManager         = (*Memory)(nil)
	_ PolicyReader          = (*Memory)(nil)
	_ AuditReader           = (*Memory)(nil)
	_ DecisionAuditReader   = (*Memory)(nil)
)

type memoryPolicy struct {
	revision       PolicyRevision
	roles          map[RoleKey]Role
	accessLayers   map[AccessLayerKey][]CapabilityKey
	automaticRules map[string]bool
	touchedScopes  map[ScopeID]struct{}
}

// NewMemory creates a reference Adapter with one root Scope and at least one
// protected administrator. Bootstrap is explicit and never reads Identity
// roles or process configuration.
func NewMemory(catalog *Catalog, options MemoryOptions) (*Memory, error) {
	if catalog == nil {
		return nil, &Error{Kind: ErrorInvalidInput, Field: "catalog", Message: "is required"}
	}
	if strings.TrimSpace(string(options.RootScopeID)) == "" {
		return nil, &Error{Kind: ErrorInvalidInput, Field: "root_scope_id", Message: "is required"}
	}
	if len(options.ProtectedSubjects) == 0 {
		return nil, &Error{Kind: ErrorInvariant, Field: "protected_subjects", Message: "at least one protected administrator is required"}
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	tokenGenerator := options.TokenGenerator
	if tokenGenerator == nil {
		tokenGenerator = secureToken
	}
	module := &Memory{
		catalog:        catalog,
		clock:          clock,
		tokenGenerator: tokenGenerator,
		scopes: map[ScopeID]Scope{
			options.RootScopeID: {ID: options.RootScopeID, Type: catalog.rootScope},
		},
		grants:           make(map[GrantID]Grant),
		groups:           make(map[GroupID]Group),
		groupMembers:     make(map[GroupID]map[string]SubjectRef),
		applications:     make(map[ApplicationID]Application),
		invitations:      make(map[InvitationID]Invitation),
		invitationTokens: make(map[[32]byte]InvitationID),
		constraints:      make(map[ConstraintKey]ConstraintEvaluator, len(options.Constraints)),
		predicates:       make(map[PredicateKey]PredicateEvaluator, len(options.Predicates)),
		inbox:            make(map[string]ReconcileResult),
		activePolicy:     1,
		nextPolicy:       1,
		policies:         make(map[uint64]*memoryPolicy),
	}
	for key := range catalog.constraints {
		evaluator, exists := options.Constraints[key]
		if !exists || evaluator == nil {
			return nil, &Error{Kind: ErrorInvalidInput, Field: "constraints", Message: fmt.Sprintf("missing evaluator for %q", key)}
		}
		module.constraints[key] = evaluator
	}
	for key := range options.Constraints {
		if _, exists := catalog.constraints[key]; !exists {
			return nil, &Error{Kind: ErrorInvalidInput, Field: "constraints", Message: fmt.Sprintf("unknown evaluator %q", key)}
		}
	}
	for _, rule := range catalog.automaticRules {
		evaluator, exists := options.Predicates[rule.Predicate]
		if !exists || evaluator == nil {
			return nil, &Error{
				Kind: ErrorInvalidInput, Field: "predicates",
				Message: fmt.Sprintf("missing evaluator for %q", rule.Predicate),
			}
		}
		module.predicates[rule.Predicate] = evaluator
	}
	for key := range options.Predicates {
		known := false
		for _, rule := range catalog.automaticRules {
			known = known || rule.Predicate == key
		}
		if !known {
			return nil, &Error{Kind: ErrorInvalidInput, Field: "predicates", Message: fmt.Sprintf("unknown evaluator %q", key)}
		}
	}
	now := clock()
	module.policies[1] = module.initialPolicyLocked(now)
	seen := make(map[string]struct{}, len(options.ProtectedSubjects))
	for index, subject := range options.ProtectedSubjects {
		if err := validateGrantSubject(subject); err != nil {
			return nil, &Error{
				Kind: ErrorInvalidInput, Field: fmt.Sprintf("protected_subjects[%d]", index),
				Message: err.Error(),
			}
		}
		key := subjectKey(subject)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		grant := Grant{
			ID: GrantID(module.newIDLocked("grant")), Target: subject, Role: catalog.protectedRole,
			RoleID:  module.policies[1].roles[catalog.protectedRole].ID,
			ScopeID: options.RootScopeID, Source: GrantSourceBootstrap, ValidFrom: now,
		}
		module.grants[grant.ID] = grant
		module.appendAuditLocked(context.Background(), AuditBootstrapProtected, SubjectRef{}, subject, grant.Role, grant.ScopeID)
	}
	return module, nil
}

func (module *Memory) CreateScope(ctx context.Context, command CreateScopeCommand) (Scope, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	now := module.clock()
	if !module.hasManageAccessLocked(command.Actor, command.ParentID, now) {
		return Scope{}, &Error{Kind: ErrorDenied, Message: "actor cannot manage the parent scope"}
	}
	if strings.TrimSpace(string(command.ID)) == "" {
		return Scope{}, &Error{Kind: ErrorInvalidInput, Field: "id", Message: "is required"}
	}
	if _, exists := module.scopes[command.ID]; exists {
		return Scope{}, &Error{Kind: ErrorConflict, Field: "id", Message: "scope already exists"}
	}
	parent, exists := module.scopes[command.ParentID]
	if !exists {
		return Scope{}, &Error{Kind: ErrorNotFound, Field: "parent_id", Message: "scope not found"}
	}
	if !module.catalog.AllowsScopeChild(parent.Type, command.Type) {
		return Scope{}, &Error{Kind: ErrorInvalidInput, Field: "type", Message: "scope edge is not allowed by the catalog"}
	}
	scope := Scope{ID: command.ID, Type: command.Type, ParentID: command.ParentID}
	module.scopes[scope.ID] = scope
	module.appendAuditLocked(ctx, AuditScopeCreated, command.Actor, SubjectRef{}, "", scope.ID)
	return scope, nil
}

func (module *Memory) RegisterScope(ctx context.Context, command RegisterScopeCommand) (Scope, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	if strings.TrimSpace(string(command.ID)) == "" {
		return Scope{}, &Error{Kind: ErrorInvalidInput, Field: "id", Message: "is required"}
	}
	if existing, exists := module.scopes[command.ID]; exists {
		if existing.Type == command.Type && existing.ParentID == command.ParentID {
			return existing, nil
		}
		return Scope{}, &Error{Kind: ErrorConflict, Field: "id", Message: "scope exists with different shape"}
	}
	parent, exists := module.scopes[command.ParentID]
	if !exists {
		return Scope{}, &Error{Kind: ErrorNotFound, Field: "parent_id", Message: "scope not found"}
	}
	if !module.catalog.AllowsScopeChild(parent.Type, command.Type) {
		return Scope{}, &Error{Kind: ErrorInvalidInput, Field: "type", Message: "scope edge is not allowed by the catalog"}
	}
	scope := Scope{ID: command.ID, Type: command.Type, ParentID: command.ParentID}
	module.scopes[scope.ID] = scope
	module.appendAuditLocked(ctx, AuditScopeRegistered, SubjectRef{}, SubjectRef{}, "", scope.ID)
	return scope, nil
}

func (module *Memory) ListScopes(_ context.Context, query ScopeListQuery) (ScopePage, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if query.Offset < 0 || query.Limit < 0 {
		return ScopePage{}, &Error{Kind: ErrorInvalidInput, Message: "offset and limit must not be negative"}
	}
	if _, exists := module.scopes[query.ScopeID]; !exists {
		return ScopePage{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	if !module.hasManageAccessLocked(query.Actor, query.ScopeID, module.clock()) {
		return ScopePage{}, &Error{Kind: ErrorDenied, Message: "actor cannot list scopes"}
	}
	var scopes []Scope
	for _, scope := range module.scopes {
		if module.scopeContainsLocked(query.ScopeID, scope.ID) {
			scopes = append(scopes, scope)
		}
	}
	sort.Slice(scopes, func(i, j int) bool { return scopes[i].ID < scopes[j].ID })
	start, end := pageBounds(len(scopes), query.Offset, query.Limit)
	page := ScopePage{Total: len(scopes)}
	if start < end {
		page.Scopes = append([]Scope(nil), scopes[start:end]...)
	}
	return page, nil
}

func (module *Memory) CreateGroup(ctx context.Context, command CreateGroupCommand) (Group, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	if !module.hasManageAccessLocked(command.Actor, command.ScopeID, module.clock()) {
		return Group{}, &Error{Kind: ErrorDenied, Message: "actor cannot create groups in the scope"}
	}
	if strings.TrimSpace(string(command.ID)) == "" {
		return Group{}, &Error{Kind: ErrorInvalidInput, Field: "id", Message: "is required"}
	}
	if strings.TrimSpace(command.DisplayName) == "" {
		return Group{}, &Error{Kind: ErrorInvalidInput, Field: "display_name", Message: "is required"}
	}
	if _, exists := module.scopes[command.ScopeID]; !exists {
		return Group{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	if _, exists := module.groups[command.ID]; exists {
		return Group{}, &Error{Kind: ErrorConflict, Field: "id", Message: "group already exists"}
	}
	group := Group{ID: command.ID, ScopeID: command.ScopeID, DisplayName: strings.TrimSpace(command.DisplayName)}
	module.groups[group.ID] = group
	module.groupMembers[group.ID] = make(map[string]SubjectRef)
	module.appendAuditLocked(ctx, AuditGroupCreated, command.Actor, SubjectRef{}, "", group.ScopeID)
	return group, nil
}

func (module *Memory) AddGroupMember(ctx context.Context, command AddGroupMemberCommand) (GroupMembership, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	group, exists := module.groups[command.GroupID]
	if !exists {
		return GroupMembership{}, &Error{Kind: ErrorNotFound, Field: "group_id", Message: "group not found"}
	}
	if !module.hasManageAccessLocked(command.Actor, group.ScopeID, module.clock()) {
		return GroupMembership{}, &Error{Kind: ErrorDenied, Message: "actor cannot manage the group"}
	}
	if command.Member.Kind == SubjectGroup {
		return GroupMembership{}, &Error{Kind: ErrorInvariant, Field: "member", Message: "nested groups are not allowed"}
	}
	if command.Member.Kind != SubjectUser && command.Member.Kind != SubjectService {
		return GroupMembership{}, &Error{Kind: ErrorInvalidInput, Field: "member", Message: "only user or service subjects may join a group"}
	}
	if strings.TrimSpace(command.Member.ID) == "" {
		return GroupMembership{}, &Error{Kind: ErrorInvalidInput, Field: "member", Message: "subject ID is required"}
	}
	key := subjectKey(command.Member)
	if _, exists := module.groupMembers[group.ID][key]; exists {
		return GroupMembership{}, &Error{Kind: ErrorConflict, Message: "group membership already exists"}
	}
	module.groupMembers[group.ID][key] = command.Member
	module.appendAuditLocked(ctx, AuditGroupMemberAdded, command.Actor, command.Member, "", group.ScopeID)
	return GroupMembership{GroupID: group.ID, Member: command.Member}, nil
}

func (module *Memory) RemoveGroupMember(ctx context.Context, command RemoveGroupMemberCommand) (GroupMembership, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	group, exists := module.groups[command.GroupID]
	if !exists {
		return GroupMembership{}, &Error{Kind: ErrorNotFound, Field: "group_id", Message: "group not found"}
	}
	if !module.hasManageAccessLocked(command.Actor, group.ScopeID, module.clock()) {
		return GroupMembership{}, &Error{Kind: ErrorDenied, Message: "actor cannot manage the group"}
	}
	key := subjectKey(command.Member)
	member, exists := module.groupMembers[group.ID][key]
	if !exists {
		return GroupMembership{}, &Error{Kind: ErrorNotFound, Field: "member", Message: "group membership not found"}
	}
	delete(module.groupMembers[group.ID], key)
	module.appendAuditLocked(ctx, AuditGroupMemberRemoved, command.Actor, member, "", group.ScopeID)
	return GroupMembership{GroupID: group.ID, Member: member}, nil
}

func (module *Memory) ListGroups(_ context.Context, query GroupListQuery) (GroupPage, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if query.Offset < 0 || query.Limit < 0 {
		return GroupPage{}, &Error{Kind: ErrorInvalidInput, Message: "offset and limit must not be negative"}
	}
	if _, exists := module.scopes[query.ScopeID]; !exists {
		return GroupPage{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	if !module.hasManageAccessLocked(query.Actor, query.ScopeID, module.clock()) {
		return GroupPage{}, &Error{Kind: ErrorDenied, Message: "actor cannot list groups"}
	}
	var groups []Group
	for _, group := range module.groups {
		if module.scopeContainsLocked(query.ScopeID, group.ScopeID) {
			groups = append(groups, group)
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	start, end := pageBounds(len(groups), query.Offset, query.Limit)
	page := GroupPage{Total: len(groups)}
	if start < end {
		page.Groups = append([]Group(nil), groups[start:end]...)
	}
	return page, nil
}

func (module *Memory) Apply(ctx context.Context, command ApplyCommand) (Application, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	if command.Actor.Kind != SubjectUser || strings.TrimSpace(command.Actor.ID) == "" {
		return Application{}, &Error{Kind: ErrorInvalidInput, Field: "actor", Message: "only an identified user may apply"}
	}
	command.Reason = strings.TrimSpace(command.Reason)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if len(command.IdempotencyKey) > 200 {
		return Application{}, &Error{Kind: ErrorInvalidInput, Field: "idempotency_key", Message: "must not exceed 200 bytes"}
	}
	if command.IdempotencyKey != "" {
		for _, application := range module.applications {
			if application.Subject != command.Actor || application.IdempotencyKey != command.IdempotencyKey {
				continue
			}
			if application.Role != command.Role || application.ScopeID != command.ScopeID ||
				application.Reason != command.Reason || application.RequestGroupID != command.RequestGroupID {
				return Application{}, &Error{
					Kind: ErrorConflict, Field: "idempotency_key",
					Message: "was already used for a different application",
				}
			}
			return application, nil
		}
	}
	role, exists := module.activeRoleLocked(command.Role)
	if !exists {
		return Application{}, &Error{Kind: ErrorInvalidInput, Field: "role", Message: "unknown role"}
	}
	if !module.scopeContainsLocked(role.ScopeID, command.ScopeID) {
		return Application{}, &Error{Kind: ErrorInvalidInput, Field: "scope_id", Message: "role is not available in this scope"}
	}
	if role.Protected || !containsGrantSource(role.Assignment.Sources, GrantSourceApplication) {
		return Application{}, &Error{Kind: ErrorInvalidInput, Field: "role", Message: "role is not requestable"}
	}
	if _, exists := module.scopes[command.ScopeID]; !exists {
		return Application{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	for _, application := range module.applications {
		if application.Subject == command.Actor && application.Role == command.Role &&
			application.ScopeID == command.ScopeID && application.State == ApplicationPending {
			return Application{}, &Error{Kind: ErrorConflict, Message: "a pending application already exists"}
		}
	}
	if len(command.Reason) > 2000 {
		return Application{}, &Error{Kind: ErrorInvalidInput, Field: "reason", Message: "must not exceed 2000 bytes"}
	}
	application := Application{
		ID: ApplicationID(module.newIDLocked("application")), RequestGroupID: command.RequestGroupID,
		Subject: command.Actor, RoleID: role.ID, Role: command.Role, ScopeID: command.ScopeID,
		Reason: command.Reason, State: ApplicationPending, CreatedAt: module.clock(),
		IdempotencyKey: command.IdempotencyKey,
	}
	module.applications[application.ID] = application
	module.appendAuditLocked(ctx, AuditApplicationCreated, command.Actor, command.Actor, command.Role, command.ScopeID)
	return application, nil
}

func (module *Memory) ReviewApplication(ctx context.Context, command ReviewApplicationCommand) (Application, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	application, exists := module.applications[command.ApplicationID]
	if !exists {
		return Application{}, &Error{Kind: ErrorNotFound, Field: "application_id", Message: "application not found"}
	}
	now := module.clock()
	if !module.hasManageAccessLocked(command.Actor, application.ScopeID, now) {
		return Application{}, &Error{Kind: ErrorDenied, Message: "actor cannot review the application"}
	}
	if application.State != ApplicationPending {
		return Application{}, &Error{Kind: ErrorConflict, Message: "application is already terminal"}
	}
	if command.Decision != ReviewApprove && command.Decision != ReviewReject {
		return Application{}, &Error{Kind: ErrorInvalidInput, Field: "decision", Message: "must be approve or reject"}
	}
	if len(command.Reason) > 2000 {
		return Application{}, &Error{Kind: ErrorInvalidInput, Field: "reason", Message: "must not exceed 2000 bytes"}
	}
	if command.Decision == ReviewApprove {
		role, exists := module.activeRoleLocked(application.Role)
		if !exists || role.ID != application.RoleID {
			return Application{}, &Error{Kind: ErrorConflict, Field: "role", Message: "role is no longer active"}
		}
		if !module.canDelegateRoleLocked(command.Actor, role, application.ScopeID, now) {
			return Application{}, &Error{Kind: ErrorDenied, Message: "actor cannot delegate the requested role"}
		}
		for _, existing := range module.grants {
			if existing.Target == application.Subject && existing.Role == application.Role &&
				existing.ScopeID == application.ScopeID && grantActive(existing, now) {
				return Application{}, &Error{Kind: ErrorConflict, Message: "an active grant already exists"}
			}
		}
		grant := Grant{
			ID: GrantID(module.newIDLocked("grant")), Target: application.Subject, Role: application.Role,
			RoleID: role.ID, ScopeID: application.ScopeID, Source: GrantSourceApplication, ValidFrom: now,
		}
		module.grants[grant.ID] = grant
		module.appendAuditLocked(ctx, AuditGrantCreated, command.Actor, grant.Target, grant.Role, grant.ScopeID)
		application.State = ApplicationApproved
		application.GrantID = grant.ID
	} else {
		application.State = ApplicationRejected
	}
	application.ReviewedAt = now
	application.ReviewedBy = command.Actor
	application.ReviewReason = strings.TrimSpace(command.Reason)
	module.applications[application.ID] = application
	module.appendAuditLocked(ctx, AuditApplicationReviewed, command.Actor, application.Subject, application.Role, application.ScopeID)
	return application, nil
}

func (module *Memory) WithdrawApplication(ctx context.Context, command WithdrawApplicationCommand) (Application, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	application, exists := module.applications[command.ApplicationID]
	if !exists {
		return Application{}, &Error{Kind: ErrorNotFound, Field: "application_id", Message: "application not found"}
	}
	if application.Subject != command.Actor {
		return Application{}, &Error{Kind: ErrorDenied, Message: "only the applicant may withdraw"}
	}
	if application.State != ApplicationPending {
		return Application{}, &Error{Kind: ErrorConflict, Message: "application is already terminal"}
	}
	application.State = ApplicationWithdrawn
	application.ReviewedAt = module.clock()
	application.ReviewedBy = command.Actor
	module.applications[application.ID] = application
	module.appendAuditLocked(ctx, AuditApplicationWithdrawn, command.Actor, application.Subject, application.Role, application.ScopeID)
	return application, nil
}

func (module *Memory) ListApplications(
	_ context.Context,
	query ApplicationListQuery,
) (ApplicationPage, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if query.Offset < 0 || query.Limit < 0 {
		return ApplicationPage{}, &Error{Kind: ErrorInvalidInput, Message: "offset and limit must not be negative"}
	}
	own := query.Subject == query.Actor && query.Actor.Kind == SubjectUser && strings.TrimSpace(query.Actor.ID) != ""
	if query.Subject == (SubjectRef{}) && query.ScopeID == "" {
		query.Subject = query.Actor
		own = query.Actor.Kind == SubjectUser && strings.TrimSpace(query.Actor.ID) != ""
	}
	if !own {
		if query.ScopeID == "" || !module.hasManageAccessLocked(query.Actor, query.ScopeID, module.clock()) {
			return ApplicationPage{}, &Error{Kind: ErrorDenied, Message: "actor cannot list the requested applications"}
		}
	}
	if query.ScopeID != "" {
		if _, exists := module.scopes[query.ScopeID]; !exists {
			return ApplicationPage{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
		}
	}
	var applications []Application
	for _, application := range module.applications {
		if query.Subject != (SubjectRef{}) && application.Subject != query.Subject {
			continue
		}
		if query.ScopeID != "" && !module.scopeContainsLocked(query.ScopeID, application.ScopeID) {
			continue
		}
		if query.State != "" && application.State != query.State {
			continue
		}
		applications = append(applications, application)
	}
	sort.Slice(applications, func(i, j int) bool {
		if applications[i].CreatedAt.Equal(applications[j].CreatedAt) {
			return applications[i].ID < applications[j].ID
		}
		return applications[i].CreatedAt.Before(applications[j].CreatedAt)
	})
	start, end := pageBounds(len(applications), query.Offset, query.Limit)
	page := ApplicationPage{Total: len(applications)}
	if start < end {
		page.Applications = append([]Application(nil), applications[start:end]...)
	}
	return page, nil
}

func (module *Memory) ListInvitations(
	_ context.Context,
	query InvitationListQuery,
) (InvitationPage, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if query.Offset < 0 || query.Limit < 0 {
		return InvitationPage{}, &Error{Kind: ErrorInvalidInput, Message: "offset and limit must not be negative"}
	}
	if _, exists := module.scopes[query.ScopeID]; !exists {
		return InvitationPage{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	if !module.hasManageAccessLocked(query.Actor, query.ScopeID, module.clock()) {
		return InvitationPage{}, &Error{Kind: ErrorDenied, Message: "actor cannot list invitations"}
	}
	var invitations []Invitation
	for _, invitation := range module.invitations {
		if !module.scopeContainsLocked(query.ScopeID, invitation.ScopeID) {
			continue
		}
		if query.State != "" && invitation.State != query.State {
			continue
		}
		invitations = append(invitations, invitation)
	}
	sort.Slice(invitations, func(i, j int) bool {
		if invitations[i].CreatedAt.Equal(invitations[j].CreatedAt) {
			return invitations[i].ID < invitations[j].ID
		}
		return invitations[i].CreatedAt.Before(invitations[j].CreatedAt)
	})
	start, end := pageBounds(len(invitations), query.Offset, query.Limit)
	page := InvitationPage{Total: len(invitations)}
	if start < end {
		page.Invitations = append([]Invitation(nil), invitations[start:end]...)
	}
	return page, nil
}

func (module *Memory) Invite(ctx context.Context, command InviteCommand) (InvitationIssue, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	now := module.clock()
	if !module.hasManageAccessLocked(command.Actor, command.ScopeID, now) {
		return InvitationIssue{}, &Error{Kind: ErrorDenied, Message: "actor cannot invite in the scope"}
	}
	role, exists := module.activeRoleLocked(command.Role)
	if !exists {
		return InvitationIssue{}, &Error{Kind: ErrorInvalidInput, Field: "role", Message: "unknown role"}
	}
	if !module.scopeContainsLocked(role.ScopeID, command.ScopeID) {
		return InvitationIssue{}, &Error{Kind: ErrorInvalidInput, Field: "scope_id", Message: "role is not available in this scope"}
	}
	if role.Protected || !containsGrantSource(role.Assignment.Sources, GrantSourceInvitation) {
		return InvitationIssue{}, &Error{Kind: ErrorInvalidInput, Field: "role", Message: "role does not allow invitations"}
	}
	if !module.canDelegateRoleLocked(command.Actor, role, command.ScopeID, now) {
		return InvitationIssue{}, &Error{Kind: ErrorDenied, Message: "actor cannot delegate the invited role"}
	}
	if _, exists := module.scopes[command.ScopeID]; !exists {
		return InvitationIssue{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	email := normalizeEmail(command.Email)
	hasSubject := command.Subject.Kind != "" || command.Subject.ID != ""
	if hasSubject == (email != "") {
		return InvitationIssue{}, &Error{Kind: ErrorInvalidInput, Message: "exactly one subject or email target is required"}
	}
	if hasSubject && (command.Subject.Kind != SubjectUser || strings.TrimSpace(command.Subject.ID) == "") {
		return InvitationIssue{}, &Error{Kind: ErrorInvalidInput, Field: "subject", Message: "invitation subject must be an identified user"}
	}
	if !command.ExpiresAt.After(now) {
		return InvitationIssue{}, &Error{Kind: ErrorInvalidInput, Field: "expires_at", Message: "must be in the future"}
	}
	for _, invitation := range module.invitations {
		sameTarget := (hasSubject && invitation.Subject == command.Subject) || (email != "" && invitation.Email == email)
		if sameTarget && invitation.Role == command.Role && invitation.ScopeID == command.ScopeID &&
			invitation.State == InvitationPending && invitation.ExpiresAt.After(now) {
			return InvitationIssue{}, &Error{Kind: ErrorConflict, Message: "a pending invitation already exists"}
		}
	}
	token, digest, err := module.issueInvitationTokenLocked()
	if err != nil {
		return InvitationIssue{}, err
	}
	invitation := Invitation{
		ID: InvitationID(module.newIDLocked("invitation")), Subject: command.Subject, Email: email,
		RoleID: role.ID, Role: command.Role, ScopeID: command.ScopeID, State: InvitationPending,
		InvitedBy: command.Actor, CreatedAt: now, ExpiresAt: command.ExpiresAt,
	}
	module.invitations[invitation.ID] = invitation
	module.invitationTokens[digest] = invitation.ID
	module.appendAuditLocked(ctx, AuditInvitationCreated, command.Actor, command.Subject, command.Role, command.ScopeID)
	return InvitationIssue{Invitation: invitation, Token: token}, nil
}

func (module *Memory) AcceptInvitation(ctx context.Context, command AcceptInvitationCommand) (Invitation, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	invitation, err := module.invitationForTokenLocked(command.Token)
	if err != nil {
		return Invitation{}, err
	}
	now := module.clock()
	if invitation.State != InvitationPending {
		return Invitation{}, &Error{Kind: ErrorConflict, Message: "invitation is already terminal"}
	}
	if !invitation.ExpiresAt.After(now) {
		invitation.State = InvitationExpired
		invitation.CompletedAt = now
		module.invitations[invitation.ID] = invitation
		return Invitation{}, &Error{Kind: ErrorExpired, Message: "invitation has expired"}
	}
	if err := invitationAcceptsActor(invitation, command.Actor, command.VerifiedEmail); err != nil {
		return Invitation{}, err
	}
	role, exists := module.activeRoleLocked(invitation.Role)
	if !exists || role.ID != invitation.RoleID {
		return Invitation{}, &Error{Kind: ErrorConflict, Field: "role", Message: "role is no longer active"}
	}
	if !module.canDelegateRoleLocked(invitation.InvitedBy, role, invitation.ScopeID, now) {
		return Invitation{}, &Error{Kind: ErrorDenied, Message: "inviter can no longer delegate the role"}
	}
	for _, existing := range module.grants {
		if existing.Target == command.Actor && existing.Role == invitation.Role &&
			existing.ScopeID == invitation.ScopeID && grantActive(existing, now) {
			return Invitation{}, &Error{Kind: ErrorConflict, Message: "an active grant already exists"}
		}
	}
	grant := Grant{
		ID: GrantID(module.newIDLocked("grant")), Target: command.Actor, Role: invitation.Role,
		RoleID: role.ID, ScopeID: invitation.ScopeID, Source: GrantSourceInvitation, ValidFrom: now,
	}
	module.grants[grant.ID] = grant
	module.appendAuditLocked(ctx, AuditGrantCreated, invitation.InvitedBy, grant.Target, grant.Role, grant.ScopeID)
	invitation.State = InvitationAccepted
	invitation.AcceptedBy = command.Actor
	invitation.GrantID = grant.ID
	invitation.CompletedAt = now
	module.invitations[invitation.ID] = invitation
	module.appendAuditLocked(ctx, AuditInvitationAccepted, command.Actor, command.Actor, invitation.Role, invitation.ScopeID)
	return invitation, nil
}

func (module *Memory) DeclineInvitation(ctx context.Context, command DeclineInvitationCommand) (Invitation, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	invitation, err := module.invitationForTokenLocked(command.Token)
	if err != nil {
		return Invitation{}, err
	}
	if invitation.State != InvitationPending {
		return Invitation{}, &Error{Kind: ErrorConflict, Message: "invitation is already terminal"}
	}
	if err := invitationAcceptsActor(invitation, command.Actor, command.VerifiedEmail); err != nil {
		return Invitation{}, err
	}
	invitation.State = InvitationDeclined
	invitation.AcceptedBy = command.Actor
	invitation.CompletedAt = module.clock()
	module.invitations[invitation.ID] = invitation
	module.appendAuditLocked(ctx, AuditInvitationDeclined, command.Actor, command.Actor, invitation.Role, invitation.ScopeID)
	return invitation, nil
}

func (module *Memory) RevokeInvitation(ctx context.Context, command RevokeInvitationCommand) (Invitation, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	invitation, exists := module.invitations[command.InvitationID]
	if !exists {
		return Invitation{}, &Error{Kind: ErrorNotFound, Field: "invitation_id", Message: "invitation not found"}
	}
	if !module.hasManageAccessLocked(command.Actor, invitation.ScopeID, module.clock()) {
		return Invitation{}, &Error{Kind: ErrorDenied, Message: "actor cannot revoke the invitation"}
	}
	if invitation.State != InvitationPending {
		return Invitation{}, &Error{Kind: ErrorConflict, Message: "invitation is already terminal"}
	}
	invitation.State = InvitationRevoked
	invitation.CompletedAt = module.clock()
	module.invitations[invitation.ID] = invitation
	module.appendAuditLocked(ctx, AuditInvitationRevoked, command.Actor, invitation.Subject, invitation.Role, invitation.ScopeID)
	return invitation, nil
}

func (module *Memory) ResendInvitation(ctx context.Context, command ResendInvitationCommand) (InvitationIssue, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	invitation, exists := module.invitations[command.InvitationID]
	if !exists {
		return InvitationIssue{}, &Error{Kind: ErrorNotFound, Field: "invitation_id", Message: "invitation not found"}
	}
	now := module.clock()
	if !module.hasManageAccessLocked(command.Actor, invitation.ScopeID, now) {
		return InvitationIssue{}, &Error{Kind: ErrorDenied, Message: "actor cannot resend the invitation"}
	}
	if invitation.State != InvitationPending {
		return InvitationIssue{}, &Error{Kind: ErrorConflict, Message: "invitation is already terminal"}
	}
	if !command.ExpiresAt.After(now) {
		return InvitationIssue{}, &Error{Kind: ErrorInvalidInput, Field: "expires_at", Message: "must be in the future"}
	}
	token, digest, err := module.issueInvitationTokenLocked()
	if err != nil {
		return InvitationIssue{}, err
	}
	for existingDigest, invitationID := range module.invitationTokens {
		if invitationID == invitation.ID {
			delete(module.invitationTokens, existingDigest)
		}
	}
	invitation.ExpiresAt = command.ExpiresAt
	module.invitations[invitation.ID] = invitation
	module.invitationTokens[digest] = invitation.ID
	module.appendAuditLocked(ctx, AuditInvitationResent, command.Actor, invitation.Subject, invitation.Role, invitation.ScopeID)
	return InvitationIssue{Invitation: invitation, Token: token}, nil
}

func (module *Memory) HandleEvent(ctx context.Context, event AutomaticEvent) (ReconcileResult, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	if strings.TrimSpace(event.ID) == "" {
		return ReconcileResult{}, &Error{Kind: ErrorInvalidInput, Field: "id", Message: "is required"}
	}
	if !qualifiedKeyPattern.MatchString(string(event.Trigger)) {
		return ReconcileResult{}, &Error{Kind: ErrorInvalidInput, Field: "trigger", Message: "must be a qualified lowercase key"}
	}
	if err := validateGrantSubject(event.Subject); err != nil || event.Subject.Kind == SubjectGroup {
		return ReconcileResult{}, &Error{Kind: ErrorInvalidInput, Field: "subject", Message: "automatic rules require a user or service subject"}
	}
	if existing, replay := module.inbox[event.ID]; replay {
		existing.Grants = slicesCloneGrants(existing.Grants)
		return existing, nil
	}
	result, err := module.reconcileSubjectLocked(ctx, event.Subject, event.Facts, event.Trigger, false)
	if err != nil {
		return ReconcileResult{}, err
	}
	module.inbox[event.ID] = ReconcileResult{
		Subject: result.Subject, Grants: slicesCloneGrants(result.Grants), Created: result.Created,
	}
	return result, nil
}

func (module *Memory) ReconcileSubject(ctx context.Context, command ReconcileSubjectCommand) (ReconcileResult, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	if err := validateGrantSubject(command.Subject); err != nil || command.Subject.Kind == SubjectGroup {
		return ReconcileResult{}, &Error{Kind: ErrorInvalidInput, Field: "subject", Message: "reconcile requires a user or service subject"}
	}
	return module.reconcileSubjectLocked(ctx, command.Subject, command.Facts, "", false)
}

func (module *Memory) PreviewReconcileSubject(
	ctx context.Context,
	command ReconcileSubjectCommand,
) (ReconcileResult, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if err := validateGrantSubject(command.Subject); err != nil || command.Subject.Kind == SubjectGroup {
		return ReconcileResult{}, &Error{Kind: ErrorInvalidInput, Field: "subject", Message: "reconcile requires a user or service subject"}
	}
	return module.reconcileSubjectLocked(ctx, command.Subject, command.Facts, "", true)
}

func (module *Memory) Backfill(ctx context.Context, commands []ReconcileSubjectCommand) ([]ReconcileResult, error) {
	results := make([]ReconcileResult, len(commands))
	for index, command := range commands {
		result, err := module.ReconcileSubject(ctx, command)
		if err != nil {
			return nil, err
		}
		results[index] = result
	}
	return results, nil
}

func (module *Memory) reconcileSubjectLocked(
	ctx context.Context,
	subject SubjectRef,
	facts map[FactKey]any,
	trigger TriggerKey,
	dryRun bool,
) (ReconcileResult, error) {
	result := ReconcileResult{Subject: subject}
	keys := make([]string, 0, len(module.catalog.automaticRules))
	for key := range module.catalog.automaticRules {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	now := module.clock()
	rootScopeID := module.rootScopeIDLocked()
	for _, key := range keys {
		rule := module.catalog.automaticRules[key]
		if !module.policies[module.activePolicy].automaticRules[key] ||
			(trigger != "" && rule.Trigger != trigger) {
			continue
		}
		evaluator := module.predicates[rule.Predicate]
		if !evaluator.Matches(ctx, PredicateInput{Subject: subject, Facts: facts}) {
			continue
		}
		var existingGrant Grant
		for _, grant := range module.grants {
			if grant.Target == subject && grant.Role == rule.Role && grant.ScopeID == rootScopeID && grantActive(grant, now) {
				existingGrant = grant
				break
			}
		}
		if existingGrant.ID != "" {
			result.Grants = append(result.Grants, existingGrant)
			continue
		}
		role, exists := module.activeRoleLocked(rule.Role)
		if !exists {
			continue
		}
		grant := Grant{
			Target: subject, RoleID: role.ID, Role: rule.Role,
			ScopeID: rootScopeID, Source: GrantSourceAutomatic, ValidFrom: now,
		}
		if dryRun {
			result.Grants = append(result.Grants, grant)
			result.Created++
			continue
		}
		grant.ID = GrantID(module.newIDLocked("grant"))
		module.grants[grant.ID] = grant
		module.appendAuditLocked(ctx, AuditGrantCreated, SubjectRef{}, subject, grant.Role, grant.ScopeID)
		result.Grants = append(result.Grants, grant)
		result.Created++
	}
	if result.Created > 0 && !dryRun {
		module.appendAuditLocked(ctx, AuditAutomaticReconciled, SubjectRef{}, subject, "", rootScopeID)
	}
	return result, nil
}

func (module *Memory) CreatePolicyDraft(ctx context.Context, command CreatePolicyDraftCommand) (PolicyRevision, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	scopeID := command.ScopeID
	if scopeID == "" {
		scopeID = module.rootScopeIDLocked()
	}
	if _, exists := module.scopes[scopeID]; !exists {
		return PolicyRevision{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	if !module.hasManageAccessLocked(command.Actor, scopeID, module.clock()) {
		return PolicyRevision{}, &Error{Kind: ErrorDenied, Message: "actor cannot manage policy in the scope"}
	}
	if command.ExpectedActiveRevision != module.activePolicy {
		return PolicyRevision{}, &Error{Kind: ErrorConflict, Message: "active policy revision changed"}
	}
	module.nextPolicy++
	policy := cloneMemoryPolicy(module.policies[module.activePolicy])
	policy.revision = PolicyRevision{
		Number: module.nextPolicy, Base: module.activePolicy, State: PolicyDraft,
		CreatedBy: command.Actor, CreatedAt: module.clock(),
	}
	policy.touchedScopes = map[ScopeID]struct{}{scopeID: {}}
	module.policies[policy.revision.Number] = policy
	module.appendAuditLocked(ctx, AuditPolicyDraftCreated, command.Actor, SubjectRef{}, "", scopeID)
	return policy.revision, nil
}

func (module *Memory) SetRoleCapabilities(
	ctx context.Context,
	command SetRoleCapabilitiesCommand,
) (PolicyRevision, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	policy, err := module.editablePolicyLocked(command.Actor, command.Revision)
	if err != nil {
		return PolicyRevision{}, err
	}
	role, exists := policy.roles[command.Role]
	if !exists {
		return PolicyRevision{}, &Error{Kind: ErrorNotFound, Field: "role", Message: "role not found"}
	}
	if role.Status != RoleActive {
		return PolicyRevision{}, &Error{Kind: ErrorConflict, Field: "role", Message: "role is retired"}
	}
	if !module.hasManageAccessLocked(command.Actor, role.ScopeID, module.clock()) {
		return PolicyRevision{}, &Error{Kind: ErrorDenied, Message: "actor cannot manage the role scope"}
	}
	if role.Protected {
		return PolicyRevision{}, &Error{Kind: ErrorInvariant, Field: "role", Message: "protected role bindings are immutable"}
	}
	capabilities, err := module.validateRoleBindingsLocked(command.Capabilities)
	if err != nil {
		return PolicyRevision{}, err
	}
	if !module.canDelegateCapabilitiesLocked(command.Actor, capabilities, role.ScopeID, module.clock()) {
		return PolicyRevision{}, &Error{Kind: ErrorDenied, Message: "actor cannot delegate every capability in the role"}
	}
	role.Capabilities = capabilities
	policy.roles[command.Role] = role
	policy.touchedScopes[role.ScopeID] = struct{}{}
	module.appendAuditLocked(ctx, AuditPolicyBindingsChanged, command.Actor, SubjectRef{}, command.Role, role.ScopeID)
	return policy.revision, nil
}

func (module *Memory) CreateRole(ctx context.Context, command CreateRoleCommand) (Role, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	policy, err := module.editablePolicyLocked(command.Actor, command.Revision)
	if err != nil {
		return Role{}, err
	}
	if !slugKeyPattern.MatchString(string(command.Key)) {
		return Role{}, &Error{Kind: ErrorInvalidInput, Field: "key", Message: "must be a lowercase slug"}
	}
	if strings.TrimSpace(command.DisplayName) == "" {
		return Role{}, &Error{Kind: ErrorInvalidInput, Field: "display_name", Message: "must not be empty"}
	}
	if _, exists := policy.roles[command.Key]; exists {
		return Role{}, &Error{Kind: ErrorConflict, Field: "key", Message: "role already exists"}
	}
	if _, exists := module.scopes[command.ScopeID]; !exists {
		return Role{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	now := module.clock()
	if !module.hasManageAccessLocked(command.Actor, command.ScopeID, now) {
		return Role{}, &Error{Kind: ErrorDenied, Message: "actor cannot manage the role scope"}
	}
	capabilities, err := module.validateRoleBindingsLocked(command.Capabilities)
	if err != nil {
		return Role{}, err
	}
	if !module.canDelegateCapabilitiesLocked(command.Actor, capabilities, command.ScopeID, now) {
		return Role{}, &Error{Kind: ErrorDenied, Message: "actor cannot delegate every capability in the role"}
	}
	assignment, err := validateCustomRoleAssignment(command.Assignment)
	if err != nil {
		return Role{}, err
	}
	role := Role{
		ID:           RoleID(module.newIDLocked("role")),
		Key:          command.Key,
		DisplayName:  strings.TrimSpace(command.DisplayName),
		ScopeID:      command.ScopeID,
		Kind:         RoleCustom,
		Status:       RoleActive,
		Capabilities: capabilities,
		Assignment:   assignment,
	}
	policy.roles[role.Key] = role
	policy.touchedScopes[role.ScopeID] = struct{}{}
	module.appendAuditLocked(ctx, AuditRoleCreated, command.Actor, SubjectRef{}, role.Key, role.ScopeID)
	return cloneRole(role), nil
}

func (module *Memory) UpdateRole(ctx context.Context, command UpdateRoleCommand) (Role, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	policy, err := module.editablePolicyLocked(command.Actor, command.Revision)
	if err != nil {
		return Role{}, err
	}
	role, exists := policy.roles[command.Role]
	if !exists {
		return Role{}, &Error{Kind: ErrorNotFound, Field: "role", Message: "role not found"}
	}
	if role.Status != RoleActive {
		return Role{}, &Error{Kind: ErrorConflict, Field: "role", Message: "role is retired"}
	}
	if role.Protected {
		return Role{}, &Error{Kind: ErrorInvariant, Field: "role", Message: "protected role is immutable"}
	}
	if strings.TrimSpace(command.DisplayName) == "" {
		return Role{}, &Error{Kind: ErrorInvalidInput, Field: "display_name", Message: "must not be empty"}
	}
	now := module.clock()
	if !module.hasManageAccessLocked(command.Actor, role.ScopeID, now) {
		return Role{}, &Error{Kind: ErrorDenied, Message: "actor cannot manage the role scope"}
	}
	capabilities, err := module.validateRoleBindingsLocked(command.Capabilities)
	if err != nil {
		return Role{}, err
	}
	if !module.canDelegateCapabilitiesLocked(command.Actor, capabilities, role.ScopeID, now) {
		return Role{}, &Error{Kind: ErrorDenied, Message: "actor cannot delegate every capability in the role"}
	}
	assignment, err := validateCustomRoleAssignment(command.Assignment)
	if err != nil {
		return Role{}, err
	}
	role.DisplayName = strings.TrimSpace(command.DisplayName)
	role.Capabilities = capabilities
	role.Assignment = assignment
	policy.roles[role.Key] = role
	policy.touchedScopes[role.ScopeID] = struct{}{}
	module.appendAuditLocked(ctx, AuditRoleUpdated, command.Actor, SubjectRef{}, role.Key, role.ScopeID)
	return cloneRole(role), nil
}

func (module *Memory) RetireRole(ctx context.Context, command RetireRoleCommand) (Role, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	policy, err := module.editablePolicyLocked(command.Actor, command.Revision)
	if err != nil {
		return Role{}, err
	}
	role, exists := policy.roles[command.Role]
	if !exists {
		return Role{}, &Error{Kind: ErrorNotFound, Field: "role", Message: "role not found"}
	}
	if role.Kind != RoleCustom || role.Protected {
		return Role{}, &Error{Kind: ErrorInvariant, Field: "role", Message: "built-in roles cannot be retired"}
	}
	if role.Status != RoleActive {
		return Role{}, &Error{Kind: ErrorConflict, Field: "role", Message: "role is already retired"}
	}
	now := module.clock()
	if !module.hasManageAccessLocked(command.Actor, role.ScopeID, now) {
		return Role{}, &Error{Kind: ErrorDenied, Message: "actor cannot manage the role scope"}
	}
	for _, grant := range module.grants {
		if grant.RoleID == role.ID && grantActive(grant, now) {
			return Role{}, &Error{Kind: ErrorConflict, Field: "role", Message: "role has active grants"}
		}
	}
	for _, application := range module.applications {
		if application.Role == role.Key && application.State == ApplicationPending {
			return Role{}, &Error{Kind: ErrorConflict, Field: "role", Message: "role has pending applications"}
		}
	}
	for _, invitation := range module.invitations {
		if invitation.Role == role.Key && invitation.State == InvitationPending && invitation.ExpiresAt.After(now) {
			return Role{}, &Error{Kind: ErrorConflict, Field: "role", Message: "role has pending invitations"}
		}
	}
	role.Status = RoleRetired
	policy.roles[role.Key] = role
	policy.touchedScopes[role.ScopeID] = struct{}{}
	module.appendAuditLocked(ctx, AuditRoleRetired, command.Actor, SubjectRef{}, role.Key, role.ScopeID)
	return cloneRole(role), nil
}

func (module *Memory) ListRoles(_ context.Context, query RoleListQuery) (RolePage, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if query.Offset < 0 || query.Limit < 0 {
		return RolePage{}, &Error{Kind: ErrorInvalidInput, Message: "offset and limit must not be negative"}
	}
	if _, exists := module.scopes[query.ScopeID]; !exists {
		return RolePage{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	if !module.hasManageAccessLocked(query.Actor, query.ScopeID, module.clock()) {
		return RolePage{}, &Error{Kind: ErrorDenied, Message: "actor cannot list roles in the scope"}
	}
	var roles []Role
	for _, role := range module.policies[module.activePolicy].roles {
		if !module.scopeContainsLocked(role.ScopeID, query.ScopeID) {
			continue
		}
		if role.Status == RoleRetired && !query.IncludeRetired {
			continue
		}
		roles = append(roles, cloneRole(role))
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Key < roles[j].Key })
	start, end := pageBounds(len(roles), query.Offset, query.Limit)
	page := RolePage{Total: len(roles)}
	if start < end {
		page.Roles = append([]Role(nil), roles[start:end]...)
	}
	return page, nil
}

func (module *Memory) ListRequestableRoles(
	_ context.Context,
	query RequestableRoleQuery,
) ([]Role, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if query.Subject.Kind != SubjectUser || strings.TrimSpace(query.Subject.ID) == "" {
		return nil, &Error{Kind: ErrorInvalidInput, Field: "subject", Message: "requestable roles require an identified user"}
	}
	if _, exists := module.scopes[query.ScopeID]; !exists {
		return nil, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	var roles []Role
	for _, role := range module.policies[module.activePolicy].roles {
		if role.Status != RoleActive || role.Protected ||
			!module.scopeContainsLocked(role.ScopeID, query.ScopeID) ||
			!containsGrantSource(role.Assignment.Sources, GrantSourceApplication) {
			continue
		}
		roles = append(roles, cloneRole(role))
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Key < roles[j].Key })
	return roles, nil
}

func (module *Memory) SetAccessLayerCapabilities(
	ctx context.Context,
	command SetAccessLayerCapabilitiesCommand,
) (PolicyRevision, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	policy, err := module.editablePolicyLocked(command.Actor, command.Revision)
	if err != nil {
		return PolicyRevision{}, err
	}
	rootScopeID := module.rootScopeIDLocked()
	if !module.hasManageAccessLocked(command.Actor, rootScopeID, module.clock()) {
		return PolicyRevision{}, &Error{Kind: ErrorDenied, Message: "actor cannot manage access layers"}
	}
	if _, exists := module.catalog.accessLayers[command.AccessLayer]; !exists {
		return PolicyRevision{}, &Error{Kind: ErrorNotFound, Field: "access_layer", Message: "access layer not found"}
	}
	seen := make(map[CapabilityKey]struct{}, len(command.Capabilities))
	capabilities := make([]CapabilityKey, 0, len(command.Capabilities))
	for _, key := range command.Capabilities {
		capability, exists := module.catalog.capabilities[key]
		if !exists {
			return PolicyRevision{}, &Error{Kind: ErrorInvalidInput, Field: "capabilities", Message: "unknown capability"}
		}
		if capability.Binding != BindingAccessLayerEligible {
			return PolicyRevision{}, &Error{Kind: ErrorInvalidInput, Field: "capabilities", Message: "capability cannot bind to an access layer"}
		}
		if _, duplicate := seen[key]; duplicate {
			return PolicyRevision{}, &Error{Kind: ErrorInvalidInput, Field: "capabilities", Message: "duplicate capability"}
		}
		seen[key] = struct{}{}
		capabilities = append(capabilities, key)
	}
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i] < capabilities[j] })
	policy.accessLayers[command.AccessLayer] = capabilities
	policy.touchedScopes[rootScopeID] = struct{}{}
	module.appendAuditLocked(ctx, AuditPolicyBindingsChanged, command.Actor, SubjectRef{}, "", rootScopeID)
	return policy.revision, nil
}

func (module *Memory) SetAutomaticRuleEnabled(
	ctx context.Context,
	command SetAutomaticRuleEnabledCommand,
) (PolicyRevision, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	policy, err := module.editablePolicyLocked(command.Actor, command.Revision)
	if err != nil {
		return PolicyRevision{}, err
	}
	definition, exists := module.catalog.automaticRules[command.Rule]
	if !exists {
		return PolicyRevision{}, &Error{Kind: ErrorNotFound, Field: "rule", Message: "automatic rule not found"}
	}
	role, exists := policy.roles[definition.Role]
	if !exists || role.Status != RoleActive {
		return PolicyRevision{}, &Error{Kind: ErrorConflict, Field: "role", Message: "automatic rule role is not active"}
	}
	now := module.clock()
	if !module.hasManageAccessLocked(command.Actor, role.ScopeID, now) {
		return PolicyRevision{}, &Error{Kind: ErrorDenied, Message: "actor cannot manage the automatic rule scope"}
	}
	if command.Enabled && !module.canDelegateRoleLocked(command.Actor, role, role.ScopeID, now) {
		return PolicyRevision{}, &Error{Kind: ErrorDenied, Message: "actor cannot delegate the automatic rule role"}
	}
	policy.automaticRules[command.Rule] = command.Enabled
	policy.touchedScopes[role.ScopeID] = struct{}{}
	module.appendAuditLocked(ctx, AuditAutomaticRuleChanged, command.Actor, SubjectRef{}, role.Key, role.ScopeID)
	return policy.revision, nil
}

func (module *Memory) ValidatePolicy(
	_ context.Context,
	command ValidatePolicyCommand,
) (PolicyValidation, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	policy, exists := module.policies[command.Revision]
	if !exists {
		return PolicyValidation{}, &Error{Kind: ErrorNotFound, Field: "revision", Message: "policy revision not found"}
	}
	if !module.canManagePolicyLocked(command.Actor, policy, module.clock()) {
		return PolicyValidation{}, &Error{Kind: ErrorDenied, Message: "actor cannot validate every changed scope"}
	}
	if policy.revision.State != PolicyDraft {
		return PolicyValidation{}, &Error{Kind: ErrorConflict, Message: "policy revision is not a draft"}
	}
	return PolicyValidation{Valid: true}, nil
}

func (module *Memory) PreviewPolicy(_ context.Context, command PreviewPolicyCommand) (PolicyImpact, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	policy, exists := module.policies[command.Revision]
	if !exists {
		return PolicyImpact{}, &Error{Kind: ErrorNotFound, Field: "revision", Message: "policy revision not found"}
	}
	if !module.canManagePolicyLocked(command.Actor, policy, module.clock()) {
		return PolicyImpact{}, &Error{Kind: ErrorDenied, Message: "actor cannot preview every changed scope"}
	}
	if policy.revision.State != PolicyDraft {
		return PolicyImpact{}, &Error{Kind: ErrorConflict, Message: "policy revision is not a draft"}
	}
	base := module.policies[policy.revision.Base]
	added, removed := bindingDiff(base, policy)
	return PolicyImpact{Revision: command.Revision, AddedBindings: added, RemovedBindings: removed}, nil
}

func (module *Memory) ActivatePolicy(ctx context.Context, command ActivatePolicyCommand) (PolicyRevision, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	if command.ExpectedActiveRevision != module.activePolicy {
		return PolicyRevision{}, &Error{Kind: ErrorConflict, Message: "active policy revision changed"}
	}
	policy, exists := module.policies[command.Revision]
	if !exists {
		return PolicyRevision{}, &Error{Kind: ErrorNotFound, Field: "revision", Message: "policy revision not found"}
	}
	if policy.revision.State != PolicyDraft || policy.revision.Base != module.activePolicy {
		return PolicyRevision{}, &Error{Kind: ErrorConflict, Message: "draft is not based on the active policy"}
	}
	if !module.canManagePolicyLocked(command.Actor, policy, module.clock()) {
		return PolicyRevision{}, &Error{Kind: ErrorDenied, Message: "actor cannot activate every changed scope"}
	}
	previous := module.policies[module.activePolicy]
	previous.revision.State = PolicySuperseded
	policy.revision.State = PolicyActive
	policy.revision.ActivatedAt = module.clock()
	module.activePolicy = policy.revision.Number
	module.appendAuditLocked(ctx, AuditPolicyActivated, command.Actor, SubjectRef{}, "", module.rootScopeIDLocked())
	return policy.revision, nil
}

func (module *Memory) RollbackPolicy(ctx context.Context, command RollbackPolicyCommand) (PolicyRevision, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	if !module.hasManageAccessLocked(command.Actor, module.rootScopeIDLocked(), module.clock()) {
		return PolicyRevision{}, &Error{Kind: ErrorDenied, Message: "actor cannot rollback root policy"}
	}
	if command.ExpectedActiveRevision != module.activePolicy {
		return PolicyRevision{}, &Error{Kind: ErrorConflict, Message: "active policy revision changed"}
	}
	source, exists := module.policies[command.SourceRevision]
	if !exists || source.revision.State == PolicyDraft || source.revision.State == PolicyDiscarded {
		return PolicyRevision{}, &Error{Kind: ErrorInvalidInput, Field: "source_revision", Message: "source policy is not an activated revision"}
	}
	previous := module.policies[module.activePolicy]
	previous.revision.State = PolicySuperseded
	module.nextPolicy++
	policy := cloneMemoryPolicy(source)
	policy.revision = PolicyRevision{
		Number: module.nextPolicy, Base: module.activePolicy, State: PolicyActive,
		CreatedBy: command.Actor, CreatedAt: module.clock(), ActivatedAt: module.clock(),
	}
	module.policies[policy.revision.Number] = policy
	module.activePolicy = policy.revision.Number
	module.appendAuditLocked(ctx, AuditPolicyRolledBack, command.Actor, SubjectRef{}, "", module.rootScopeIDLocked())
	return policy.revision, nil
}

func (module *Memory) ListPolicyRevisions(
	_ context.Context,
	query PolicyRevisionListQuery,
) (PolicyRevisionPage, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if query.Offset < 0 || query.Limit < 0 {
		return PolicyRevisionPage{}, &Error{Kind: ErrorInvalidInput, Message: "offset and limit must not be negative"}
	}
	if _, exists := module.scopes[query.ScopeID]; !exists {
		return PolicyRevisionPage{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	if !module.hasManageAccessLocked(query.Actor, query.ScopeID, module.clock()) {
		return PolicyRevisionPage{}, &Error{Kind: ErrorDenied, Message: "actor cannot list policy revisions"}
	}
	var revisions []PolicyRevision
	for _, policy := range module.policies {
		visible := len(policy.touchedScopes) == 0
		for scopeID := range policy.touchedScopes {
			if module.scopeContainsLocked(query.ScopeID, scopeID) {
				visible = true
				break
			}
		}
		if visible {
			revisions = append(revisions, policy.revision)
		}
	}
	sort.Slice(revisions, func(i, j int) bool { return revisions[i].Number < revisions[j].Number })
	start, end := pageBounds(len(revisions), query.Offset, query.Limit)
	page := PolicyRevisionPage{Total: len(revisions)}
	if start < end {
		page.Revisions = append([]PolicyRevision(nil), revisions[start:end]...)
	}
	return page, nil
}

func (module *Memory) GetPolicySnapshot(
	_ context.Context,
	query PolicySnapshotQuery,
) (PolicySnapshot, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if _, exists := module.scopes[query.ScopeID]; !exists {
		return PolicySnapshot{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	if !module.hasManageAccessLocked(query.Actor, query.ScopeID, module.clock()) {
		return PolicySnapshot{}, &Error{Kind: ErrorDenied, Message: "actor cannot read policy"}
	}
	policy, exists := module.policies[query.Revision]
	if !exists {
		return PolicySnapshot{}, &Error{Kind: ErrorNotFound, Field: "revision", Message: "policy revision not found"}
	}
	snapshot := PolicySnapshot{Revision: policy.revision}
	for _, role := range policy.roles {
		if module.scopeContainsLocked(role.ScopeID, query.ScopeID) {
			snapshot.Roles = append(snapshot.Roles, cloneRole(role))
		}
	}
	sort.Slice(snapshot.Roles, func(i, j int) bool { return snapshot.Roles[i].Key < snapshot.Roles[j].Key })
	for key, capabilities := range policy.accessLayers {
		snapshot.AccessLayers = append(snapshot.AccessLayers, AccessLayerPolicy{
			Key: key, Capabilities: append([]CapabilityKey(nil), capabilities...),
		})
	}
	sort.Slice(snapshot.AccessLayers, func(i, j int) bool {
		return snapshot.AccessLayers[i].Key < snapshot.AccessLayers[j].Key
	})
	for key, enabled := range policy.automaticRules {
		snapshot.AutomaticRules = append(snapshot.AutomaticRules, AutomaticRulePolicy{Key: key, Enabled: enabled})
	}
	sort.Slice(snapshot.AutomaticRules, func(i, j int) bool {
		return snapshot.AutomaticRules[i].Key < snapshot.AutomaticRules[j].Key
	})
	return snapshot, nil
}

func (module *Memory) Grant(ctx context.Context, command GrantCommand) (Grant, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	now := module.clock()
	if !module.hasManageAccessLocked(command.Actor, command.ScopeID, now) {
		return Grant{}, &Error{Kind: ErrorDenied, Message: "actor cannot grant roles in the scope"}
	}
	if err := validateGrantSubject(command.Target); err != nil {
		return Grant{}, &Error{Kind: ErrorInvalidInput, Field: "target", Message: err.Error()}
	}
	role, exists := module.activeRoleLocked(command.Role)
	if !exists {
		return Grant{}, &Error{Kind: ErrorInvalidInput, Field: "role", Message: "unknown role"}
	}
	if _, exists := module.scopes[command.ScopeID]; !exists {
		return Grant{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	if !module.scopeContainsLocked(role.ScopeID, command.ScopeID) {
		return Grant{}, &Error{Kind: ErrorInvalidInput, Field: "scope_id", Message: "role is not available in this scope"}
	}
	if role.Protected && command.ScopeID != module.rootScopeIDLocked() {
		return Grant{}, &Error{Kind: ErrorInvariant, Field: "scope_id", Message: "protected role can only be granted at the root scope"}
	}
	actorProtected := module.hasProtectedAccessLocked(command.Actor, command.ScopeID, now)
	if role.Protected && !actorProtected {
		return Grant{}, &Error{Kind: ErrorDenied, Message: "only a protected administrator may grant the protected role"}
	}
	if !role.Protected && !module.canDelegateRoleLocked(command.Actor, role, command.ScopeID, now) {
		return Grant{}, &Error{Kind: ErrorDenied, Message: "actor cannot delegate every capability in the role"}
	}
	if role.Protected && command.Target.Kind == SubjectGroup {
		return Grant{}, &Error{Kind: ErrorInvariant, Field: "target", Message: "protected role cannot be granted to a group"}
	}
	if command.Target.Kind == SubjectGroup {
		groupID := GroupID(command.Target.ID)
		group, exists := module.groups[groupID]
		if !exists {
			return Grant{}, &Error{Kind: ErrorNotFound, Field: "target", Message: "group not found"}
		}
		if command.Source != GrantSourceGroup {
			return Grant{}, &Error{Kind: ErrorInvalidInput, Field: "source", Message: "group target requires group source"}
		}
		if !module.scopeContainsLocked(group.ScopeID, command.ScopeID) {
			return Grant{}, &Error{Kind: ErrorInvariant, Field: "scope_id", Message: "group grant cannot escape its scope"}
		}
	} else if command.Source == GrantSourceGroup {
		return Grant{}, &Error{Kind: ErrorInvalidInput, Field: "source", Message: "group source requires a group target"}
	}
	if !role.Protected && !containsGrantSource(role.Assignment.Sources, command.Source) {
		return Grant{}, &Error{Kind: ErrorInvalidInput, Field: "source", Message: "role does not allow this assignment source"}
	}
	if role.Protected && command.Source != GrantSourceDirect {
		return Grant{}, &Error{Kind: ErrorInvalidInput, Field: "source", Message: "protected role only allows direct assignment after bootstrap"}
	}
	validFrom := command.ValidFrom
	if validFrom.IsZero() {
		validFrom = now
	}
	if !command.ExpiresAt.IsZero() && !command.ExpiresAt.After(validFrom) {
		return Grant{}, &Error{Kind: ErrorInvalidInput, Field: "expires_at", Message: "must be after valid_from"}
	}
	if role.Assignment.MaxDuration > 0 && !command.ExpiresAt.IsZero() && command.ExpiresAt.Sub(validFrom) > role.Assignment.MaxDuration {
		return Grant{}, &Error{Kind: ErrorInvalidInput, Field: "expires_at", Message: "exceeds role assignment policy"}
	}
	for _, existing := range module.grants {
		if existing.Target == command.Target && existing.Role == command.Role && existing.ScopeID == command.ScopeID && grantActive(existing, now) {
			return Grant{}, &Error{Kind: ErrorConflict, Message: "an active grant already exists"}
		}
	}
	grant := Grant{
		ID: GrantID(module.newIDLocked("grant")), Target: command.Target, RoleID: role.ID, Role: command.Role,
		ScopeID: command.ScopeID, Source: command.Source, ValidFrom: validFrom, ExpiresAt: command.ExpiresAt,
	}
	module.grants[grant.ID] = grant
	module.appendAuditLocked(ctx, AuditGrantCreated, command.Actor, grant.Target, grant.Role, grant.ScopeID)
	return grant, nil
}

func (module *Memory) Revoke(ctx context.Context, command RevokeCommand) (Grant, error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	now := module.clock()
	grant, exists := module.grants[command.GrantID]
	if !exists {
		return Grant{}, &Error{Kind: ErrorNotFound, Field: "grant_id", Message: "grant not found"}
	}
	if !module.hasManageAccessLocked(command.Actor, grant.ScopeID, now) {
		return Grant{}, &Error{Kind: ErrorDenied, Message: "actor cannot revoke the grant"}
	}
	role, exists := module.activeRoleLocked(grant.Role)
	if !exists || role.ID != grant.RoleID {
		return Grant{}, &Error{Kind: ErrorConflict, Field: "role", Message: "grant role is no longer active"}
	}
	actorProtected := module.hasProtectedAccessLocked(command.Actor, grant.ScopeID, now)
	if role.Protected && !actorProtected {
		return Grant{}, &Error{Kind: ErrorDenied, Message: "only a protected administrator may revoke the protected role"}
	}
	if grant.Target == command.Actor && containsCapability(module.roleCapabilitiesLocked(grant.Role), CapabilityManage) {
		return Grant{}, &Error{Kind: ErrorInvariant, Message: "actor cannot revoke the grant that provides their management authority"}
	}
	if !grantActive(grant, now) {
		return Grant{}, &Error{Kind: ErrorConflict, Message: "grant is not active"}
	}
	if grant.Role == module.catalog.protectedRole && module.activeProtectedCountLocked(now) <= 1 {
		return Grant{}, &Error{Kind: ErrorInvariant, Message: "cannot revoke the last protected administrator"}
	}
	grant.RevokedAt = now
	module.grants[grant.ID] = grant
	module.appendAuditLocked(ctx, AuditGrantRevoked, command.Actor, grant.Target, grant.Role, grant.ScopeID)
	return grant, nil
}

func (module *Memory) ListGrants(_ context.Context, query GrantListQuery) (GrantPage, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if query.Offset < 0 || query.Limit < 0 {
		return GrantPage{}, &Error{Kind: ErrorInvalidInput, Message: "offset and limit must not be negative"}
	}
	if _, exists := module.scopes[query.ScopeID]; !exists {
		return GrantPage{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	if !module.hasManageAccessLocked(query.Actor, query.ScopeID, module.clock()) {
		return GrantPage{}, &Error{Kind: ErrorDenied, Message: "actor cannot list grants"}
	}
	now := module.clock()
	var grants []Grant
	for _, grant := range module.grants {
		if !module.scopeContainsLocked(query.ScopeID, grant.ScopeID) {
			continue
		}
		if query.Target != (SubjectRef{}) && grant.Target != query.Target {
			continue
		}
		if query.Role != "" && grant.Role != query.Role {
			continue
		}
		if query.ActiveOnly && !grantActive(grant, now) {
			continue
		}
		grants = append(grants, grant)
	}
	sort.Slice(grants, func(i, j int) bool { return grants[i].ID < grants[j].ID })
	start, end := pageBounds(len(grants), query.Offset, query.Limit)
	page := GrantPage{Total: len(grants)}
	if start < end {
		page.Grants = append([]Grant(nil), grants[start:end]...)
	}
	return page, nil
}

func (module *Memory) Decide(ctx context.Context, request DecisionRequest) (decision Decision, err error) {
	module.mu.Lock()
	defer module.mu.Unlock()
	decision = Decision{
		ID: DecisionID(module.newIDLocked("decision")), PolicyRevision: module.activePolicy,
	}
	defer func() {
		if err == nil {
			module.appendDecisionAuditLocked(ctx, request, decision)
		}
	}()
	if err := validateDecisionSubject(request.Subject); err != nil {
		return Decision{}, &Error{Kind: ErrorInvalidInput, Field: "subject", Message: err.Error()}
	}
	capability, exists := module.catalog.capabilities[request.Capability]
	if !exists {
		return Decision{}, &Error{Kind: ErrorInvalidInput, Field: "capability", Message: "unknown capability"}
	}
	scope, exists := module.scopes[request.ScopeID]
	if !exists {
		return Decision{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	if !capabilityAllowedAtScope(capability, scope.Type) {
		return Decision{}, &Error{Kind: ErrorInvalidInput, Field: "scope_id", Message: "capability is not valid at this scope type"}
	}
	if request.Resource.ScopeID != "" && request.Resource.ScopeID != request.ScopeID {
		return Decision{}, &Error{Kind: ErrorInvalidInput, Field: "resource.scope_id", Message: "does not match the request scope"}
	}
	if !subjectEligible(capability.EligibleSubjects, request.Subject.Kind) {
		decision.Reason = ReasonNoGrant
		return decision, nil
	}
	if deniedBy, denied := module.evaluateConstraintsLocked(ctx, ConstraintGlobal, request, DecisionSource{}); denied {
		decision.Reason = ReasonConstraint
		decision.Constraint = deniedBy
		return decision, nil
	}

	var sources []DecisionSource
	layerKey := AccessLayerAuthenticated
	if request.Subject.Kind == SubjectAnonymous {
		layerKey = AccessLayerVisitor
	}
	if _, ok := module.catalog.accessLayers[layerKey]; ok &&
		containsCapability(module.accessLayerCapabilitiesLocked(layerKey), request.Capability) {
		sources = append(sources, DecisionSource{Kind: ReasonAccessLayer, AccessLayer: layerKey})
	}
	now := module.clock()
	for _, grant := range module.grants {
		groupID, targetMatches := module.grantAppliesToSubjectLocked(grant, request.Subject)
		if !targetMatches || !grantActive(grant, now) || !module.scopeContainsLocked(grant.ScopeID, request.ScopeID) {
			continue
		}
		role, exists := module.activeRoleLocked(grant.Role)
		if !exists || role.ID != grant.RoleID {
			continue
		}
		candidateAllowed := role.Protected || containsCapability(module.roleCapabilitiesLocked(grant.Role), request.Capability)
		if module.casbin != nil {
			var err error
			candidateAllowed, err = module.casbin.allows(request.Subject, role.ID, request.ScopeID, request.Capability)
			if err != nil {
				return Decision{}, &Error{
					Kind: ErrorUnavailable, Field: "execution_projection",
					Message: "Casbin candidate evaluation failed", err: err,
				}
			}
		}
		if candidateAllowed {
			sources = append(sources, DecisionSource{
				Kind: ReasonRoleGrant, RoleID: grant.RoleID, Role: grant.Role, GrantID: grant.ID, GroupID: groupID,
			})
		}
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Role == sources[j].Role {
			return sources[i].GrantID < sources[j].GrantID
		}
		return sources[i].Role < sources[j].Role
	})
	var firstDenied ConstraintKey
	allowedSources := sources[:0]
	for _, source := range sources {
		if deniedBy, denied := module.evaluateConstraintsLocked(ctx, ConstraintSource, request, source); denied {
			if firstDenied == "" {
				firstDenied = deniedBy
			}
			continue
		}
		allowedSources = append(allowedSources, source)
	}
	sources = allowedSources
	decision.Sources = sources
	if len(sources) == 0 {
		if firstDenied != "" {
			decision.Reason = ReasonConstraint
			decision.Constraint = firstDenied
			return decision, nil
		}
		decision.Reason = ReasonNoGrant
		return decision, nil
	}
	decision.Allowed = true
	decision.Reason = sources[0].Kind
	return decision, nil
}

func (module *Memory) evaluateConstraintsLocked(
	ctx context.Context,
	mode ConstraintMode,
	request DecisionRequest,
	source DecisionSource,
) (ConstraintKey, bool) {
	keys := make([]ConstraintKey, 0, len(module.catalog.constraints))
	for key := range module.catalog.constraints {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, key := range keys {
		definition := module.catalog.constraints[key]
		if definition.Mode != mode || !containsCapability(definition.Capabilities, request.Capability) {
			continue
		}
		if mode == ConstraintSource {
			roleMatches := source.Role != "" && containsRole(definition.Roles, source.Role)
			if source.Role != "" && definition.AllNormalRoles {
				if role, exists := module.activeRoleLocked(source.Role); exists && !role.Protected && role.ID == source.RoleID {
					roleMatches = true
				}
			}
			layerMatches := source.AccessLayer != "" && containsAccessLayer(definition.AccessLayers, source.AccessLayer)
			if !roleMatches && !layerMatches {
				continue
			}
		}
		result := module.constraints[key].Evaluate(ctx, ConstraintInput{
			Subject: request.Subject, Capability: request.Capability, ScopeID: request.ScopeID,
			Resource: request.Resource, Source: source,
		})
		if result.Denied {
			return key, true
		}
	}
	return "", false
}

func (module *Memory) BatchDecide(ctx context.Context, requests []DecisionRequest) ([]Decision, error) {
	decisions := make([]Decision, len(requests))
	for index, request := range requests {
		decision, err := module.Decide(ctx, request)
		if err != nil {
			return nil, err
		}
		decisions[index] = decision
	}
	return decisions, nil
}

func (module *Memory) Plan(_ context.Context, request QueryRequest) (QueryConstraint, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if err := validateDecisionSubject(request.Subject); err != nil {
		return QueryConstraint{}, &Error{Kind: ErrorInvalidInput, Field: "subject", Message: err.Error()}
	}
	capability, exists := module.catalog.capabilities[request.Capability]
	if !exists {
		return QueryConstraint{}, &Error{Kind: ErrorInvalidInput, Field: "capability", Message: "unknown capability"}
	}
	scope, exists := module.scopes[request.ScopeID]
	if !exists {
		return QueryConstraint{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	if !capabilityAllowedAtScope(capability, scope.Type) {
		return QueryConstraint{}, &Error{Kind: ErrorInvalidInput, Field: "scope_id", Message: "capability is not valid at this scope type"}
	}
	if !subjectEligible(capability.EligibleSubjects, request.Subject.Kind) {
		return QueryConstraint{Kind: QueryNone}, nil
	}
	if module.hasApplicableGlobalConstraintLocked(request.Capability) {
		return QueryConstraint{}, &Error{
			Kind: ErrorUnavailable, Field: "capability",
			Message: "global constraint cannot be represented by the query planner",
		}
	}

	layerKey := AccessLayerAuthenticated
	if request.Subject.Kind == SubjectAnonymous {
		layerKey = AccessLayerVisitor
	}
	if _, ok := module.catalog.accessLayers[layerKey]; ok &&
		containsCapability(module.accessLayerCapabilitiesLocked(layerKey), request.Capability) {
		if module.sourceNeedsUnsupportedQueryConstraintLocked(request.Capability, DecisionSource{
			Kind: ReasonAccessLayer, AccessLayer: layerKey,
		}) {
			return QueryConstraint{}, &Error{Kind: ErrorUnavailable, Message: "access-layer constraint cannot be represented"}
		}
		return QueryConstraint{Kind: QueryAll}, nil
	}

	now := module.clock()
	hasNormalRole := false
	for _, grant := range module.grants {
		groupID, targetMatches := module.grantAppliesToSubjectLocked(grant, request.Subject)
		if !targetMatches || !grantActive(grant, now) || !module.scopeContainsLocked(grant.ScopeID, request.ScopeID) {
			continue
		}
		role, exists := module.activeRoleLocked(grant.Role)
		if !exists || role.ID != grant.RoleID {
			continue
		}
		if role.Protected {
			return QueryConstraint{Kind: QueryAll}, nil
		}
		if !containsCapability(module.roleCapabilitiesLocked(grant.Role), request.Capability) {
			continue
		}
		source := DecisionSource{
			Kind: ReasonRoleGrant, RoleID: grant.RoleID, Role: grant.Role, GrantID: grant.ID, GroupID: groupID,
		}
		if module.sourceNeedsUnsupportedQueryConstraintLocked(request.Capability, source) && capability.QueryableRelation == "" {
			return QueryConstraint{}, &Error{Kind: ErrorUnavailable, Message: "role constraint cannot be represented"}
		}
		hasNormalRole = true
	}
	if !hasNormalRole {
		return QueryConstraint{Kind: QueryNone}, nil
	}
	if capability.QueryableRelation != "" {
		return QueryConstraint{
			Kind: QueryRelation, Subject: request.Subject, Relation: capability.QueryableRelation,
		}, nil
	}
	return QueryConstraint{Kind: QueryAll}, nil
}

func (module *Memory) hasApplicableGlobalConstraintLocked(capability CapabilityKey) bool {
	for _, definition := range module.catalog.constraints {
		if definition.Mode == ConstraintGlobal && containsCapability(definition.Capabilities, capability) {
			return true
		}
	}
	return false
}

func (module *Memory) sourceNeedsUnsupportedQueryConstraintLocked(
	capability CapabilityKey,
	source DecisionSource,
) bool {
	for _, definition := range module.catalog.constraints {
		if definition.Mode != ConstraintSource || !containsCapability(definition.Capabilities, capability) {
			continue
		}
		if source.Role != "" && containsRole(definition.Roles, source.Role) {
			return true
		}
		if source.Role != "" && definition.AllNormalRoles {
			if role, exists := module.activeRoleLocked(source.Role); exists && !role.Protected && role.ID == source.RoleID {
				return true
			}
		}
		if source.AccessLayer != "" && containsAccessLayer(definition.AccessLayers, source.AccessLayer) {
			return true
		}
	}
	return false
}

func (module *Memory) EffectiveAccess(_ context.Context, query EffectiveAccessQuery) (EffectiveAccess, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if err := validateDecisionSubject(query.Subject); err != nil {
		return EffectiveAccess{}, &Error{Kind: ErrorInvalidInput, Field: "subject", Message: err.Error()}
	}
	scope, exists := module.scopes[query.ScopeID]
	if !exists {
		return EffectiveAccess{}, &Error{Kind: ErrorNotFound, Field: "scope_id", Message: "scope not found"}
	}
	now := module.clock()
	result := EffectiveAccess{Subject: query.Subject, ScopeID: query.ScopeID}
	capabilities := make(map[CapabilityKey]struct{})
	layerKey := AccessLayerAuthenticated
	if query.Subject.Kind == SubjectAnonymous {
		layerKey = AccessLayerVisitor
	}
	if _, exists := module.catalog.accessLayers[layerKey]; exists {
		for _, capabilityKey := range module.accessLayerCapabilitiesLocked(layerKey) {
			capability := module.catalog.capabilities[capabilityKey]
			if subjectEligible(capability.EligibleSubjects, query.Subject.Kind) &&
				capabilityAllowedAtScope(capability, scope.Type) {
				capabilities[capabilityKey] = struct{}{}
			}
		}
	}
	for _, grant := range module.grants {
		_, targetMatches := module.grantAppliesToSubjectLocked(grant, query.Subject)
		if !targetMatches || !grantActive(grant, now) || !module.scopeContainsLocked(grant.ScopeID, query.ScopeID) {
			continue
		}
		result.Grants = append(result.Grants, grant)
		role, exists := module.activeRoleLocked(grant.Role)
		if !exists || role.ID != grant.RoleID {
			continue
		}
		if role.Protected {
			for capabilityKey, capability := range module.catalog.capabilities {
				if subjectEligible(capability.EligibleSubjects, query.Subject.Kind) &&
					capabilityAllowedAtScope(capability, scope.Type) {
					capabilities[capabilityKey] = struct{}{}
				}
			}
			continue
		}
		for _, capabilityKey := range module.roleCapabilitiesLocked(grant.Role) {
			capability := module.catalog.capabilities[capabilityKey]
			if subjectEligible(capability.EligibleSubjects, query.Subject.Kind) &&
				capabilityAllowedAtScope(capability, scope.Type) {
				capabilities[capabilityKey] = struct{}{}
			}
		}
	}
	for capability := range capabilities {
		result.Capabilities = append(result.Capabilities, capability)
	}
	sort.Slice(result.Grants, func(i, j int) bool { return result.Grants[i].ID < result.Grants[j].ID })
	sort.Slice(result.Capabilities, func(i, j int) bool { return result.Capabilities[i] < result.Capabilities[j] })
	return result, nil
}

func (module *Memory) SearchAudit(_ context.Context, query AuditQuery) (AuditPage, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if query.Offset < 0 || query.Limit < 0 {
		return AuditPage{}, &Error{Kind: ErrorInvalidInput, Message: "audit offset and limit cannot be negative"}
	}
	limit := query.Limit
	if limit == 0 {
		limit = 100
	}
	if limit > 500 {
		return AuditPage{}, &Error{Kind: ErrorInvalidInput, Field: "limit", Message: "must not exceed 500"}
	}
	matches := make([]AuditEvent, 0)
	for _, event := range module.audit {
		if query.Actor.Kind != "" && event.Actor != query.Actor {
			continue
		}
		if query.Subject.Kind != "" && event.Subject != query.Subject {
			continue
		}
		if query.Action != "" && event.Action != query.Action {
			continue
		}
		if query.Role != "" && event.Role != query.Role {
			continue
		}
		if query.ScopeID != "" && event.ScopeID != query.ScopeID {
			continue
		}
		if query.CorrelationID != "" && event.CorrelationID != query.CorrelationID {
			continue
		}
		matches = append(matches, event)
	}
	page := AuditPage{Total: len(matches)}
	if query.Offset >= len(matches) {
		return page, nil
	}
	end := min(query.Offset+limit, len(matches))
	page.Events = append([]AuditEvent(nil), matches[query.Offset:end]...)
	return page, nil
}

func (module *Memory) SearchDecisionAudit(
	_ context.Context,
	query DecisionAuditQuery,
) (DecisionAuditPage, error) {
	module.mu.RLock()
	defer module.mu.RUnlock()
	if query.Offset < 0 || query.Limit < 0 {
		return DecisionAuditPage{}, &Error{Kind: ErrorInvalidInput, Message: "offset and limit must not be negative"}
	}
	limit := query.Limit
	if limit == 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	matches := make([]DecisionAuditEvent, 0, len(module.decisionAudit))
	for _, event := range module.decisionAudit {
		if query.Subject != (SubjectRef{}) && event.Subject != query.Subject {
			continue
		}
		if query.Capability != "" && event.Capability != query.Capability {
			continue
		}
		if query.Allowed != nil && event.Allowed != *query.Allowed {
			continue
		}
		if query.CorrelationID != "" && event.CorrelationID != query.CorrelationID {
			continue
		}
		event.Sources = append([]DecisionSource(nil), event.Sources...)
		matches = append(matches, event)
	}
	page := DecisionAuditPage{Total: len(matches)}
	if query.Offset >= len(matches) {
		return page, nil
	}
	end := min(query.Offset+limit, len(matches))
	page.Events = append([]DecisionAuditEvent(nil), matches[query.Offset:end]...)
	return page, nil
}

func (module *Memory) appendDecisionAuditLocked(ctx context.Context, request DecisionRequest, decision Decision) {
	capability, exists := module.catalog.capabilities[request.Capability]
	if !exists {
		return
	}
	record := !decision.Allowed || capability.Risk == RiskHigh || capability.Audit == AuditFull
	if decision.Allowed && !record {
		for _, source := range decision.Sources {
			if source.Role == "" {
				continue
			}
			if role, exists := module.activeRoleLocked(source.Role); exists && role.Protected && role.ID == source.RoleID {
				record = true
				break
			}
		}
	}
	if !record {
		return
	}
	module.decisionAudit = append(module.decisionAudit, DecisionAuditEvent{
		DecisionID: decision.ID, Subject: request.Subject, Capability: request.Capability,
		ScopeID: request.ScopeID, ResourceType: request.Resource.Type, ResourceID: request.Resource.ID,
		ResourceRevision: request.Resource.Revision, Allowed: decision.Allowed, Reason: decision.Reason,
		Constraint: decision.Constraint, PolicyRevision: decision.PolicyRevision,
		CorrelationID: RequestMetadataFromContext(ctx).CorrelationID,
		Sources:       append([]DecisionSource(nil), decision.Sources...), OccurredAt: module.clock(),
	})
}

func (module *Memory) appendAuditLocked(
	ctx context.Context,
	action AuditAction,
	actor SubjectRef,
	subject SubjectRef,
	role RoleKey,
	scopeID ScopeID,
) {
	module.audit = append(module.audit, AuditEvent{
		ID: AuditID(module.newIDLocked("audit")), Action: action, Actor: actor,
		Subject: subject, Role: role, ScopeID: scopeID,
		PolicyRevision: module.activePolicy,
		CorrelationID:  RequestMetadataFromContext(ctx).CorrelationID,
		OccurredAt:     module.clock(),
	})
}

func (module *Memory) grantAppliesToSubjectLocked(grant Grant, subject SubjectRef) (GroupID, bool) {
	if grant.Target == subject {
		return "", true
	}
	if grant.Target.Kind != SubjectGroup {
		return "", false
	}
	groupID := GroupID(grant.Target.ID)
	_, exists := module.groupMembers[groupID][subjectKey(subject)]
	return groupID, exists
}

func (module *Memory) initialPolicyLocked(now time.Time) *memoryPolicy {
	rootScopeID := module.rootScopeIDLocked()
	policy := &memoryPolicy{
		revision: PolicyRevision{
			Number: 1, State: PolicyActive, CreatedAt: now, ActivatedAt: now,
		},
		roles:          make(map[RoleKey]Role, len(module.catalog.roles)),
		accessLayers:   make(map[AccessLayerKey][]CapabilityKey, len(module.catalog.accessLayers)),
		automaticRules: make(map[string]bool, len(module.catalog.automaticRules)),
		touchedScopes:  make(map[ScopeID]struct{}),
	}
	for key, role := range module.catalog.roles {
		policy.roles[key] = Role{
			ID:           RoleID("builtin:" + string(key)),
			Key:          key,
			DisplayName:  role.DisplayName,
			ScopeID:      rootScopeID,
			Kind:         RoleBuiltin,
			Status:       RoleActive,
			Protected:    role.Protected,
			Capabilities: append([]CapabilityKey(nil), role.Capabilities...),
			Assignment: AssignmentPolicy{
				Sources:     append([]GrantSource(nil), role.Assignment.Sources...),
				MaxDuration: role.Assignment.MaxDuration,
			},
		}
	}
	for key, layer := range module.catalog.accessLayers {
		policy.accessLayers[key] = append([]CapabilityKey(nil), layer.Capabilities...)
	}
	for key, rule := range module.catalog.automaticRules {
		policy.automaticRules[key] = rule.Enabled
	}
	return policy
}

func cloneMemoryPolicy(source *memoryPolicy) *memoryPolicy {
	clone := &memoryPolicy{
		revision:       source.revision,
		roles:          make(map[RoleKey]Role, len(source.roles)),
		accessLayers:   make(map[AccessLayerKey][]CapabilityKey, len(source.accessLayers)),
		automaticRules: make(map[string]bool, len(source.automaticRules)),
		touchedScopes:  make(map[ScopeID]struct{}, len(source.touchedScopes)),
	}
	for key, role := range source.roles {
		role.Capabilities = append([]CapabilityKey(nil), role.Capabilities...)
		role.Assignment.Sources = append([]GrantSource(nil), role.Assignment.Sources...)
		clone.roles[key] = role
	}
	for key, capabilities := range source.accessLayers {
		clone.accessLayers[key] = append([]CapabilityKey(nil), capabilities...)
	}
	for key, enabled := range source.automaticRules {
		clone.automaticRules[key] = enabled
	}
	for scopeID := range source.touchedScopes {
		clone.touchedScopes[scopeID] = struct{}{}
	}
	return clone
}

func (module *Memory) editablePolicyLocked(_ SubjectRef, revision uint64) (*memoryPolicy, error) {
	policy, exists := module.policies[revision]
	if !exists {
		return nil, &Error{Kind: ErrorNotFound, Field: "revision", Message: "policy revision not found"}
	}
	if policy.revision.State != PolicyDraft {
		return nil, &Error{Kind: ErrorConflict, Message: "policy revision is not a draft"}
	}
	return policy, nil
}

func (module *Memory) canManagePolicyLocked(actor SubjectRef, policy *memoryPolicy, now time.Time) bool {
	for scopeID := range policy.touchedScopes {
		if !module.hasManageAccessLocked(actor, scopeID, now) {
			return false
		}
	}
	return len(policy.touchedScopes) > 0
}

func (module *Memory) validateRoleBindingsLocked(bindings []CapabilityKey) ([]CapabilityKey, error) {
	seen := make(map[CapabilityKey]struct{}, len(bindings))
	capabilities := make([]CapabilityKey, 0, len(bindings))
	for _, key := range bindings {
		capability, exists := module.catalog.capabilities[key]
		if !exists {
			return nil, &Error{Kind: ErrorInvalidInput, Field: "capabilities", Message: "unknown capability"}
		}
		if capability.Binding != BindingNormal {
			return nil, &Error{Kind: ErrorInvalidInput, Field: "capabilities", Message: "capability cannot bind to a normal role"}
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, &Error{Kind: ErrorInvalidInput, Field: "capabilities", Message: "duplicate capability"}
		}
		seen[key] = struct{}{}
		capabilities = append(capabilities, key)
	}
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i] < capabilities[j] })
	return capabilities, nil
}

func validateCustomRoleAssignment(assignment AssignmentPolicy) (AssignmentPolicy, error) {
	if len(assignment.Sources) == 0 {
		return AssignmentPolicy{}, &Error{Kind: ErrorInvalidInput, Field: "assignment.sources", Message: "at least one source is required"}
	}
	if assignment.MaxDuration < 0 {
		return AssignmentPolicy{}, &Error{Kind: ErrorInvalidInput, Field: "assignment.max_duration", Message: "must not be negative"}
	}
	seen := make(map[GrantSource]struct{}, len(assignment.Sources))
	sources := make([]GrantSource, 0, len(assignment.Sources))
	for _, source := range assignment.Sources {
		if !validGrantSource(source) || source == GrantSourceBootstrap {
			return AssignmentPolicy{}, &Error{Kind: ErrorInvalidInput, Field: "assignment.sources", Message: "source is not available to custom roles"}
		}
		if _, duplicate := seen[source]; duplicate {
			return AssignmentPolicy{}, &Error{Kind: ErrorInvalidInput, Field: "assignment.sources", Message: "duplicate source"}
		}
		seen[source] = struct{}{}
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i] < sources[j] })
	return AssignmentPolicy{Sources: sources, MaxDuration: assignment.MaxDuration}, nil
}

func cloneRole(role Role) Role {
	role.Capabilities = append([]CapabilityKey(nil), role.Capabilities...)
	role.Assignment.Sources = append([]GrantSource(nil), role.Assignment.Sources...)
	return role
}

func bindingDiff(base, target *memoryPolicy) (int, int) {
	baseSet := policyBindingSet(base)
	targetSet := policyBindingSet(target)
	added, removed := 0, 0
	for binding := range targetSet {
		if _, exists := baseSet[binding]; !exists {
			added++
		}
	}
	for binding := range baseSet {
		if _, exists := targetSet[binding]; !exists {
			removed++
		}
	}
	return added, removed
}

func policyBindingSet(policy *memoryPolicy) map[string]struct{} {
	set := make(map[string]struct{})
	for roleKey, role := range policy.roles {
		if role.Status != RoleActive {
			continue
		}
		for _, capability := range role.Capabilities {
			set["role\x00"+string(roleKey)+"\x00"+string(capability)] = struct{}{}
		}
	}
	for layer, capabilities := range policy.accessLayers {
		for _, capability := range capabilities {
			set["layer\x00"+string(layer)+"\x00"+string(capability)] = struct{}{}
		}
	}
	for rule, enabled := range policy.automaticRules {
		if enabled {
			set["automatic\x00"+rule] = struct{}{}
		}
	}
	return set
}

func (module *Memory) roleCapabilitiesLocked(role RoleKey) []CapabilityKey {
	return module.policies[module.activePolicy].roles[role].Capabilities
}

func (module *Memory) activeRoleLocked(roleKey RoleKey) (Role, bool) {
	role, exists := module.policies[module.activePolicy].roles[roleKey]
	return role, exists && role.Status == RoleActive
}

func (module *Memory) accessLayerCapabilitiesLocked(layer AccessLayerKey) []CapabilityKey {
	return module.policies[module.activePolicy].accessLayers[layer]
}

func (module *Memory) hasManageAccessLocked(subject SubjectRef, scopeID ScopeID, now time.Time) bool {
	return module.hasCapabilityAccessLocked(subject, CapabilityManage, scopeID, now)
}

func (module *Memory) hasCapabilityAccessLocked(
	subject SubjectRef,
	capability CapabilityKey,
	scopeID ScopeID,
	now time.Time,
) bool {
	if err := validateGrantSubject(subject); err != nil {
		return false
	}
	for _, grant := range module.grants {
		_, targetMatches := module.grantAppliesToSubjectLocked(grant, subject)
		if !targetMatches || !grantActive(grant, now) || !module.scopeContainsLocked(grant.ScopeID, scopeID) {
			continue
		}
		role, exists := module.activeRoleLocked(grant.Role)
		if !exists || role.ID != grant.RoleID {
			continue
		}
		if role.Protected || containsCapability(module.roleCapabilitiesLocked(grant.Role), capability) {
			return true
		}
	}
	return false
}

func (module *Memory) canDelegateRoleLocked(
	actor SubjectRef,
	role Role,
	scopeID ScopeID,
	now time.Time,
) bool {
	if module.hasProtectedAccessLocked(actor, scopeID, now) {
		return true
	}
	if role.Protected {
		return false
	}
	return module.canDelegateCapabilitiesLocked(actor, role.Capabilities, scopeID, now)
}

func (module *Memory) canDelegateCapabilitiesLocked(
	actor SubjectRef,
	capabilities []CapabilityKey,
	scopeID ScopeID,
	now time.Time,
) bool {
	if module.hasProtectedAccessLocked(actor, scopeID, now) {
		return true
	}
	for _, capabilityKey := range capabilities {
		capability := module.catalog.capabilities[capabilityKey]
		if !capability.Delegable || !module.hasCapabilityAccessLocked(actor, capabilityKey, scopeID, now) {
			return false
		}
	}
	return true
}

func (module *Memory) hasProtectedAccessLocked(subject SubjectRef, scopeID ScopeID, now time.Time) bool {
	if err := validateGrantSubject(subject); err != nil {
		return false
	}
	for _, grant := range module.grants {
		if grant.Target == subject && grant.Role == module.catalog.protectedRole &&
			grantActive(grant, now) && module.scopeContainsLocked(grant.ScopeID, scopeID) {
			return true
		}
	}
	return false
}

func (module *Memory) activeProtectedCountLocked(now time.Time) int {
	count := 0
	for _, grant := range module.grants {
		if grant.Role == module.catalog.protectedRole && grantActive(grant, now) {
			count++
		}
	}
	return count
}

func (module *Memory) scopeContainsLocked(ancestor, descendant ScopeID) bool {
	for current := descendant; current != ""; {
		if current == ancestor {
			return true
		}
		scope, exists := module.scopes[current]
		if !exists {
			return false
		}
		current = scope.ParentID
	}
	return false
}

func (module *Memory) rootScopeIDLocked() ScopeID {
	for id, scope := range module.scopes {
		if scope.ParentID == "" {
			return id
		}
	}
	return ""
}

func (module *Memory) newIDLocked(prefix string) string {
	module.nextID++
	return fmt.Sprintf("%s-%d", prefix, module.nextID)
}

func validateDecisionSubject(subject SubjectRef) error {
	if subject.Kind == SubjectAnonymous {
		if subject.ID != "" {
			return fmt.Errorf("anonymous subject must not have an ID")
		}
		return nil
	}
	if subject.Kind != SubjectGuest && subject.Kind != SubjectUser && subject.Kind != SubjectService {
		return fmt.Errorf("subject kind %q cannot make access decisions", subject.Kind)
	}
	if strings.TrimSpace(subject.ID) == "" {
		return fmt.Errorf("subject ID is required")
	}
	return nil
}

func validateGrantSubject(subject SubjectRef) error {
	if subject.Kind != SubjectUser && subject.Kind != SubjectService && subject.Kind != SubjectGroup {
		return fmt.Errorf("subject kind %q cannot receive a role grant", subject.Kind)
	}
	if strings.TrimSpace(subject.ID) == "" {
		return fmt.Errorf("subject ID is required")
	}
	return nil
}

func subjectKey(subject SubjectRef) string { return string(subject.Kind) + "\x00" + subject.ID }

func grantActive(grant Grant, now time.Time) bool {
	return !grant.ValidFrom.After(now) &&
		(grant.ExpiresAt.IsZero() || grant.ExpiresAt.After(now)) &&
		grant.RevokedAt.IsZero()
}

func containsGrantSource(sources []GrantSource, source GrantSource) bool {
	for _, candidate := range sources {
		if candidate == source {
			return true
		}
	}
	return false
}

func containsCapability(capabilities []CapabilityKey, capability CapabilityKey) bool {
	for _, candidate := range capabilities {
		if candidate == capability {
			return true
		}
	}
	return false
}

func containsRole(roles []RoleKey, role RoleKey) bool {
	for _, candidate := range roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func containsAccessLayer(layers []AccessLayerKey, layer AccessLayerKey) bool {
	for _, candidate := range layers {
		if candidate == layer {
			return true
		}
	}
	return false
}

func pageBounds(total, offset, requestedLimit int) (int, int) {
	if offset >= total {
		return total, total
	}
	limit := requestedLimit
	if limit == 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	return offset, min(offset+limit, total)
}

func subjectEligible(eligible []SubjectKind, kind SubjectKind) bool {
	if len(eligible) == 0 {
		return true
	}
	for _, candidate := range eligible {
		if candidate == kind {
			return true
		}
	}
	return false
}

func capabilityAllowedAtScope(capability CapabilityDefinition, scopeType ScopeType) bool {
	if len(capability.AllowedScopes) == 0 {
		return true
	}
	for _, candidate := range capability.AllowedScopes {
		if candidate == scopeType {
			return true
		}
	}
	return false
}

func (module *Memory) issueInvitationTokenLocked() (string, [32]byte, error) {
	token, err := module.tokenGenerator()
	if err != nil {
		return "", [32]byte{}, &Error{Kind: ErrorUnavailable, Message: "cannot generate invitation token", err: err}
	}
	if strings.TrimSpace(token) == "" {
		return "", [32]byte{}, &Error{Kind: ErrorUnavailable, Message: "invitation token generator returned an empty token"}
	}
	digest := sha256.Sum256([]byte(token))
	if _, collision := module.invitationTokens[digest]; collision {
		return "", [32]byte{}, &Error{Kind: ErrorUnavailable, Message: "invitation token collision"}
	}
	return token, digest, nil
}

func (module *Memory) invitationForTokenLocked(token string) (Invitation, error) {
	if strings.TrimSpace(token) == "" {
		return Invitation{}, &Error{Kind: ErrorInvalidInput, Field: "token", Message: "is required"}
	}
	digest := sha256.Sum256([]byte(token))
	invitationID, exists := module.invitationTokens[digest]
	if !exists {
		return Invitation{}, &Error{Kind: ErrorNotFound, Message: "invitation not found"}
	}
	return module.invitations[invitationID], nil
}

func invitationAcceptsActor(invitation Invitation, actor SubjectRef, verifiedEmail string) error {
	if actor.Kind != SubjectUser || strings.TrimSpace(actor.ID) == "" {
		return &Error{Kind: ErrorInvalidInput, Field: "actor", Message: "an identified user is required"}
	}
	if invitation.Subject.Kind != "" {
		if invitation.Subject != actor {
			return &Error{Kind: ErrorDenied, Message: "invitation belongs to another subject"}
		}
		return nil
	}
	if normalized := normalizeEmail(verifiedEmail); normalized == "" || normalized != invitation.Email {
		return &Error{Kind: ErrorDenied, Message: "verified email does not match the invitation"}
	}
	return nil
}

func normalizeEmail(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func secureToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func slicesCloneGrants(grants []Grant) []Grant {
	return append([]Grant(nil), grants...)
}
