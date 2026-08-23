package authorization

import (
	"context"
	"time"
)

type ScopeID string
type GrantID string
type DecisionID string
type GroupID string
type RoleID string

type ReasonCode string

const (
	ReasonNoGrant     ReasonCode = "no_grant"
	ReasonRoleGrant   ReasonCode = "role_grant"
	ReasonAccessLayer ReasonCode = "access_layer"
	ReasonConstraint  ReasonCode = "constraint"
)

type Scope struct {
	ID       ScopeID
	Type     ScopeType
	ParentID ScopeID
}

type CreateScopeCommand struct {
	Actor    SubjectRef
	ID       ScopeID
	Type     ScopeType
	ParentID ScopeID
}

type Grant struct {
	ID        GrantID
	Target    SubjectRef
	RoleID    RoleID
	Role      RoleKey
	ScopeID   ScopeID
	Source    GrantSource
	ValidFrom time.Time
	ExpiresAt time.Time
	RevokedAt time.Time
}

type GrantCommand struct {
	Actor     SubjectRef
	Target    SubjectRef
	Role      RoleKey
	ScopeID   ScopeID
	Source    GrantSource
	ValidFrom time.Time
	ExpiresAt time.Time
}

type RevokeCommand struct {
	Actor   SubjectRef
	GrantID GrantID
}

type DecisionRequest struct {
	Subject    SubjectRef
	Capability CapabilityKey
	ScopeID    ScopeID
	Resource   ResourceFacts
}

type DecisionSource struct {
	Kind        ReasonCode
	RoleID      RoleID
	Role        RoleKey
	AccessLayer AccessLayerKey
	GrantID     GrantID
	GroupID     GroupID
}

type Decision struct {
	ID             DecisionID
	Allowed        bool
	Reason         ReasonCode
	PolicyRevision uint64
	Sources        []DecisionSource
	Constraint     ConstraintKey
}

type EffectiveAccessQuery struct {
	Subject SubjectRef
	ScopeID ScopeID
}

type EffectiveAccess struct {
	Subject      SubjectRef
	ScopeID      ScopeID
	Capabilities []CapabilityKey
	Grants       []Grant
}

type ResourceType string
type ResourceID string
type RelationKind string
type AttributeKey string

type ResourceFacts struct {
	Type       ResourceType
	ID         ResourceID
	ScopeID    ScopeID
	Relations  map[RelationKind][]SubjectRef
	Attributes map[AttributeKey]any
	Revision   string
}

type ConstraintMode string

const (
	ConstraintGlobal ConstraintMode = "global"
	ConstraintSource ConstraintMode = "source"
)

type ConstraintInput struct {
	Subject    SubjectRef
	Capability CapabilityKey
	ScopeID    ScopeID
	Resource   ResourceFacts
	Source     DecisionSource
}

type ConstraintResult struct {
	Denied bool
}

// ConstraintEvaluator is a code-owned, typed safety rule.
type ConstraintEvaluator interface {
	Evaluate(context.Context, ConstraintInput) ConstraintResult
}

// ConstraintFunc adapts a function to ConstraintEvaluator.
type ConstraintFunc func(context.Context, ConstraintInput) ConstraintResult

func (function ConstraintFunc) Evaluate(ctx context.Context, input ConstraintInput) ConstraintResult {
	return function(ctx, input)
}

// Authorizer is the stable decision Interface used by request handlers.
type Authorizer interface {
	Decide(context.Context, DecisionRequest) (Decision, error)
	BatchDecide(context.Context, []DecisionRequest) ([]Decision, error)
}

type QueryRequest struct {
	Subject    SubjectRef
	Capability CapabilityKey
	ScopeID    ScopeID
}

type QueryKind string

const (
	QueryAll      QueryKind = "all"
	QueryNone     QueryKind = "none"
	QueryRelation QueryKind = "relation"
	QueryScopeIn  QueryKind = "scope_in"
	QueryAny      QueryKind = "any"
	QueryAllOf    QueryKind = "all_of"
)

// QueryConstraint is a closed, transport-neutral AST. Consumers translate it
// through a resource-specific field mapping; it never contains SQL.
type QueryConstraint struct {
	Kind     QueryKind
	Subject  SubjectRef
	Relation RelationKind
	Scopes   []ScopeID
	Children []QueryConstraint
}

type QueryPlanner interface {
	Plan(context.Context, QueryRequest) (QueryConstraint, error)
}

type AccessReader interface {
	EffectiveAccess(context.Context, EffectiveAccessQuery) (EffectiveAccess, error)
}

type ScopeManager interface {
	CreateScope(context.Context, CreateScopeCommand) (Scope, error)
}

// RegisterScopeCommand is emitted by a trusted consumer resource lifecycle.
// Registering a scope never grants access; it only makes an existing product
// resource addressable by authorization decisions.
type RegisterScopeCommand struct {
	ID       ScopeID
	Type     ScopeType
	ParentID ScopeID
}

// ReparentScopeCommand is emitted by a trusted consumer when an existing
// domain resource moves to a different parent in the declared scope schema.
// It changes reachability only; grants remain attached to their scope IDs.
type ReparentScopeCommand struct {
	ID       ScopeID
	ParentID ScopeID
}

// ResourceScopeRegistry is deliberately separate from the administrator-facing
// ScopeManager. Consumers call it after creating domain resources or during an
// idempotent backfill, without giving end users authorization.manage.
type ResourceScopeRegistry interface {
	RegisterScope(context.Context, RegisterScopeCommand) (Scope, error)
}

// ResourceScopeRelocator is the optional trusted lifecycle seam for consumers
// whose resources can move between parents after creation.
type ResourceScopeRelocator interface {
	ReparentScope(context.Context, ReparentScopeCommand) (Scope, error)
}

type ScopeListQuery struct {
	Actor   SubjectRef
	ScopeID ScopeID
	Offset  int
	Limit   int
}

type ScopePage struct {
	Scopes []Scope
	Total  int
}

type ScopeReader interface {
	ListScopes(context.Context, ScopeListQuery) (ScopePage, error)
}

type RoleKind string

const (
	RoleBuiltin RoleKind = "builtin"
	RoleCustom  RoleKind = "custom"
)

type RoleStatus string

const (
	RoleActive  RoleStatus = "active"
	RoleRetired RoleStatus = "retired"
)

// Role is the policy-owned runtime role. Built-in roles originate in the
// consumer Definition; custom roles are created within a Scope.
type Role struct {
	ID           RoleID
	Key          RoleKey
	DisplayName  string
	ScopeID      ScopeID
	Kind         RoleKind
	Status       RoleStatus
	Protected    bool
	Capabilities []CapabilityKey
	Assignment   AssignmentPolicy
}

type CreateRoleCommand struct {
	Actor        SubjectRef
	Revision     uint64
	Key          RoleKey
	DisplayName  string
	ScopeID      ScopeID
	Capabilities []CapabilityKey
	Assignment   AssignmentPolicy
}

type UpdateRoleCommand struct {
	Actor        SubjectRef
	Revision     uint64
	Role         RoleKey
	DisplayName  string
	Capabilities []CapabilityKey
	Assignment   AssignmentPolicy
}

type RetireRoleCommand struct {
	Actor    SubjectRef
	Revision uint64
	Role     RoleKey
}

type RoleManager interface {
	CreateRole(context.Context, CreateRoleCommand) (Role, error)
	UpdateRole(context.Context, UpdateRoleCommand) (Role, error)
	RetireRole(context.Context, RetireRoleCommand) (Role, error)
}

type RoleListQuery struct {
	Actor          SubjectRef
	ScopeID        ScopeID
	IncludeRetired bool
	Offset         int
	Limit          int
}

type RolePage struct {
	Roles []Role
	Total int
}

type RequestableRoleQuery struct {
	Subject SubjectRef
	ScopeID ScopeID
}

type RoleReader interface {
	ListRoles(context.Context, RoleListQuery) (RolePage, error)
	ListRequestableRoles(context.Context, RequestableRoleQuery) ([]Role, error)
}

type GrantManager interface {
	Grant(context.Context, GrantCommand) (Grant, error)
	Revoke(context.Context, RevokeCommand) (Grant, error)
}

type GrantListQuery struct {
	Actor      SubjectRef
	ScopeID    ScopeID
	Target     SubjectRef
	Role       RoleKey
	ActiveOnly bool
	Offset     int
	Limit      int
}

type GrantPage struct {
	Grants []Grant
	Total  int
}

type GrantReader interface {
	ListGrants(context.Context, GrantListQuery) (GrantPage, error)
}

type AdministratorClaimStatus struct {
	Claimed bool
}

type ClaimInitialAdministratorCommand struct {
	Actor SubjectRef
}

type ClaimInitialAdministratorResult struct {
	Status  AdministratorClaimStatus
	Grant   Grant
	Created bool
}

// AdministratorClaimer is the one-time initialization interface for an
// authorization instance created without bootstrap administrators.
type AdministratorClaimer interface {
	AdministratorClaimStatus(context.Context) (AdministratorClaimStatus, error)
	ClaimInitialAdministrator(context.Context, ClaimInitialAdministratorCommand) (ClaimInitialAdministratorResult, error)
}

type Group struct {
	ID          GroupID
	ScopeID     ScopeID
	DisplayName string
}

type GroupMembership struct {
	GroupID GroupID
	Member  SubjectRef
}

type CreateGroupCommand struct {
	Actor       SubjectRef
	ID          GroupID
	ScopeID     ScopeID
	DisplayName string
}

type AddGroupMemberCommand struct {
	Actor   SubjectRef
	GroupID GroupID
	Member  SubjectRef
}

type RemoveGroupMemberCommand struct {
	Actor   SubjectRef
	GroupID GroupID
	Member  SubjectRef
}

type GroupManager interface {
	CreateGroup(context.Context, CreateGroupCommand) (Group, error)
	AddGroupMember(context.Context, AddGroupMemberCommand) (GroupMembership, error)
	RemoveGroupMember(context.Context, RemoveGroupMemberCommand) (GroupMembership, error)
}

type GroupListQuery struct {
	Actor   SubjectRef
	ScopeID ScopeID
	Offset  int
	Limit   int
}

type GroupPage struct {
	Groups []Group
	Total  int
}

type GroupReader interface {
	ListGroups(context.Context, GroupListQuery) (GroupPage, error)
}

type ApplicationID string
type RequestGroupID string

type ApplicationState string

const (
	ApplicationPending   ApplicationState = "pending"
	ApplicationApproved  ApplicationState = "approved"
	ApplicationRejected  ApplicationState = "rejected"
	ApplicationWithdrawn ApplicationState = "withdrawn"
	ApplicationExpired   ApplicationState = "expired"
)

type Application struct {
	ID             ApplicationID
	RequestGroupID RequestGroupID
	Subject        SubjectRef
	RoleID         RoleID
	Role           RoleKey
	ScopeID        ScopeID
	Reason         string
	State          ApplicationState
	GrantID        GrantID
	CreatedAt      time.Time
	ReviewedAt     time.Time
	ReviewedBy     SubjectRef
	ReviewReason   string
	IdempotencyKey string
}

type ApplyCommand struct {
	Actor          SubjectRef
	Role           RoleKey
	ScopeID        ScopeID
	Reason         string
	RequestGroupID RequestGroupID
	IdempotencyKey string
}

type ReviewDecision string

const (
	ReviewApprove ReviewDecision = "approve"
	ReviewReject  ReviewDecision = "reject"
)

type ReviewApplicationCommand struct {
	Actor         SubjectRef
	ApplicationID ApplicationID
	Decision      ReviewDecision
	Reason        string
}

type WithdrawApplicationCommand struct {
	Actor         SubjectRef
	ApplicationID ApplicationID
}

type WorkflowManager interface {
	Apply(context.Context, ApplyCommand) (Application, error)
	ReviewApplication(context.Context, ReviewApplicationCommand) (Application, error)
	WithdrawApplication(context.Context, WithdrawApplicationCommand) (Application, error)
	Invite(context.Context, InviteCommand) (InvitationIssue, error)
	AcceptInvitation(context.Context, AcceptInvitationCommand) (Invitation, error)
	DeclineInvitation(context.Context, DeclineInvitationCommand) (Invitation, error)
	RevokeInvitation(context.Context, RevokeInvitationCommand) (Invitation, error)
	ResendInvitation(context.Context, ResendInvitationCommand) (InvitationIssue, error)
}

type ApplicationListQuery struct {
	Actor   SubjectRef
	Subject SubjectRef
	ScopeID ScopeID
	State   ApplicationState
	Offset  int
	Limit   int
}

type ApplicationPage struct {
	Applications []Application
	Total        int
}

type WorkflowReader interface {
	ListApplications(context.Context, ApplicationListQuery) (ApplicationPage, error)
	ListInvitations(context.Context, InvitationListQuery) (InvitationPage, error)
}

type InvitationListQuery struct {
	Actor   SubjectRef
	ScopeID ScopeID
	State   InvitationState
	Offset  int
	Limit   int
}

type InvitationPage struct {
	Invitations []Invitation
	Total       int
}

type InvitationID string

type InvitationState string

const (
	InvitationPending  InvitationState = "pending"
	InvitationAccepted InvitationState = "accepted"
	InvitationDeclined InvitationState = "declined"
	InvitationRevoked  InvitationState = "revoked"
	InvitationExpired  InvitationState = "expired"
)

type Invitation struct {
	ID          InvitationID
	Subject     SubjectRef
	Email       string
	RoleID      RoleID
	Role        RoleKey
	ScopeID     ScopeID
	State       InvitationState
	InvitedBy   SubjectRef
	AcceptedBy  SubjectRef
	GrantID     GrantID
	CreatedAt   time.Time
	ExpiresAt   time.Time
	CompletedAt time.Time
}

// InvitationIssue exposes a plaintext token exactly when it is issued or
// rotated. Implementations retain only its digest.
type InvitationIssue struct {
	Invitation Invitation
	Token      string
}

type InviteCommand struct {
	Actor     SubjectRef
	Subject   SubjectRef
	Email     string
	Role      RoleKey
	ScopeID   ScopeID
	ExpiresAt time.Time
}

type AcceptInvitationCommand struct {
	Actor         SubjectRef
	VerifiedEmail string
	Token         string
}

type DeclineInvitationCommand struct {
	Actor         SubjectRef
	VerifiedEmail string
	Token         string
}

type RevokeInvitationCommand struct {
	Actor        SubjectRef
	InvitationID InvitationID
}

type ResendInvitationCommand struct {
	Actor        SubjectRef
	InvitationID InvitationID
	ExpiresAt    time.Time
}

type FactKey string

type PredicateInput struct {
	Subject SubjectRef
	Facts   map[FactKey]any
}

type PredicateEvaluator interface {
	Matches(context.Context, PredicateInput) bool
}

type PredicateFunc func(context.Context, PredicateInput) bool

func (function PredicateFunc) Matches(ctx context.Context, input PredicateInput) bool {
	return function(ctx, input)
}

type AutomaticEvent struct {
	ID      string
	Trigger TriggerKey
	Subject SubjectRef
	Facts   map[FactKey]any
}

type ReconcileSubjectCommand struct {
	Subject SubjectRef
	Facts   map[FactKey]any
}

type ReconcileResult struct {
	Subject SubjectRef
	Grants  []Grant
	Created int
}

type Reconciler interface {
	HandleEvent(context.Context, AutomaticEvent) (ReconcileResult, error)
	PreviewReconcileSubject(context.Context, ReconcileSubjectCommand) (ReconcileResult, error)
	ReconcileSubject(context.Context, ReconcileSubjectCommand) (ReconcileResult, error)
	Backfill(context.Context, []ReconcileSubjectCommand) ([]ReconcileResult, error)
}

type PolicyState string

const (
	PolicyDraft      PolicyState = "draft"
	PolicyActive     PolicyState = "active"
	PolicySuperseded PolicyState = "superseded"
	PolicyDiscarded  PolicyState = "discarded"
)

type PolicyRevision struct {
	Number      uint64
	Base        uint64
	State       PolicyState
	CreatedBy   SubjectRef
	CreatedAt   time.Time
	ActivatedAt time.Time
}

type CreatePolicyDraftCommand struct {
	Actor                  SubjectRef
	ScopeID                ScopeID
	ExpectedActiveRevision uint64
}

type SetRoleCapabilitiesCommand struct {
	Actor        SubjectRef
	Revision     uint64
	Role         RoleKey
	Capabilities []CapabilityKey
}

type SetAccessLayerCapabilitiesCommand struct {
	Actor        SubjectRef
	Revision     uint64
	AccessLayer  AccessLayerKey
	Capabilities []CapabilityKey
}

type SetAutomaticRuleEnabledCommand struct {
	Actor    SubjectRef
	Revision uint64
	Rule     string
	Enabled  bool
}

type ValidatePolicyCommand struct {
	Actor    SubjectRef
	Revision uint64
}

type PolicyValidation struct {
	Valid      bool
	Violations []string
}

type PreviewPolicyCommand struct {
	Actor    SubjectRef
	Revision uint64
}

type PolicyImpact struct {
	Revision        uint64
	AddedBindings   int
	RemovedBindings int
}

type ActivatePolicyCommand struct {
	Actor                  SubjectRef
	Revision               uint64
	ExpectedActiveRevision uint64
}

type RollbackPolicyCommand struct {
	Actor                  SubjectRef
	SourceRevision         uint64
	ExpectedActiveRevision uint64
}

type PolicyManager interface {
	CreatePolicyDraft(context.Context, CreatePolicyDraftCommand) (PolicyRevision, error)
	SetRoleCapabilities(context.Context, SetRoleCapabilitiesCommand) (PolicyRevision, error)
	SetAccessLayerCapabilities(context.Context, SetAccessLayerCapabilitiesCommand) (PolicyRevision, error)
	SetAutomaticRuleEnabled(context.Context, SetAutomaticRuleEnabledCommand) (PolicyRevision, error)
	ValidatePolicy(context.Context, ValidatePolicyCommand) (PolicyValidation, error)
	PreviewPolicy(context.Context, PreviewPolicyCommand) (PolicyImpact, error)
	ActivatePolicy(context.Context, ActivatePolicyCommand) (PolicyRevision, error)
	RollbackPolicy(context.Context, RollbackPolicyCommand) (PolicyRevision, error)
}

type PolicyRevisionListQuery struct {
	Actor   SubjectRef
	ScopeID ScopeID
	Offset  int
	Limit   int
}

type PolicyRevisionPage struct {
	Revisions []PolicyRevision
	Total     int
}

type AccessLayerPolicy struct {
	Key          AccessLayerKey
	Capabilities []CapabilityKey
}

type AutomaticRulePolicy struct {
	Key     string
	Enabled bool
}

// PolicySnapshot is the transport-neutral, read-only representation needed by
// management surfaces. It deliberately excludes Adapter projection details.
type PolicySnapshot struct {
	Revision       PolicyRevision
	Roles          []Role
	AccessLayers   []AccessLayerPolicy
	AutomaticRules []AutomaticRulePolicy
}

type PolicySnapshotQuery struct {
	Actor    SubjectRef
	Revision uint64
	ScopeID  ScopeID
}

type PolicyReader interface {
	ListPolicyRevisions(context.Context, PolicyRevisionListQuery) (PolicyRevisionPage, error)
	GetPolicySnapshot(context.Context, PolicySnapshotQuery) (PolicySnapshot, error)
}

type AuditID string
type AuditAction string

const (
	AuditBootstrapProtected          AuditAction = "bootstrap.protected"
	AuditInitialAdministratorClaimed AuditAction = "initial_administrator.claimed"
	AuditScopeCreated                AuditAction = "scope.created"
	AuditScopeRegistered             AuditAction = "scope.registered"
	AuditScopeReparented             AuditAction = "scope.reparented"
	AuditGroupCreated                AuditAction = "group.created"
	AuditGroupMemberAdded            AuditAction = "group.member_added"
	AuditGroupMemberRemoved          AuditAction = "group.member_removed"
	AuditGrantCreated                AuditAction = "grant.created"
	AuditGrantRevoked                AuditAction = "grant.revoked"
	AuditApplicationCreated          AuditAction = "application.created"
	AuditApplicationReviewed         AuditAction = "application.reviewed"
	AuditApplicationWithdrawn        AuditAction = "application.withdrawn"
	AuditInvitationCreated           AuditAction = "invitation.created"
	AuditInvitationAccepted          AuditAction = "invitation.accepted"
	AuditInvitationDeclined          AuditAction = "invitation.declined"
	AuditInvitationRevoked           AuditAction = "invitation.revoked"
	AuditInvitationResent            AuditAction = "invitation.resent"
	AuditAutomaticReconciled         AuditAction = "automatic.reconciled"
	AuditRoleCreated                 AuditAction = "role.created"
	AuditRoleUpdated                 AuditAction = "role.updated"
	AuditRoleRetired                 AuditAction = "role.retired"
	AuditAutomaticRuleChanged        AuditAction = "automatic.rule_changed"
	AuditPolicyDraftCreated          AuditAction = "policy.draft_created"
	AuditPolicyBindingsChanged       AuditAction = "policy.bindings_changed"
	AuditPolicyActivated             AuditAction = "policy.activated"
	AuditPolicyRolledBack            AuditAction = "policy.rolled_back"
	AuditRecoveryProtected           AuditAction = "recovery.protected"
)

type AuditEvent struct {
	ID             AuditID
	Action         AuditAction
	Actor          SubjectRef
	Subject        SubjectRef
	Role           RoleKey
	ScopeID        ScopeID
	PolicyRevision uint64
	CorrelationID  string
	OccurredAt     time.Time
}

type AuditQuery struct {
	Actor         SubjectRef
	Subject       SubjectRef
	Action        AuditAction
	Role          RoleKey
	ScopeID       ScopeID
	CorrelationID string
	Offset        int
	Limit         int
}

type AuditPage struct {
	Events []AuditEvent
	Total  int
}

type AuditReader interface {
	SearchAudit(context.Context, AuditQuery) (AuditPage, error)
}

type DecisionAuditEvent struct {
	DecisionID       DecisionID
	Subject          SubjectRef
	Capability       CapabilityKey
	ScopeID          ScopeID
	ResourceType     ResourceType
	ResourceID       ResourceID
	ResourceRevision string
	Allowed          bool
	Reason           ReasonCode
	Constraint       ConstraintKey
	PolicyRevision   uint64
	CorrelationID    string
	Sources          []DecisionSource
	OccurredAt       time.Time
}

type DecisionAuditQuery struct {
	Subject       SubjectRef
	Capability    CapabilityKey
	Allowed       *bool
	CorrelationID string
	Offset        int
	Limit         int
}

type DecisionAuditPage struct {
	Events []DecisionAuditEvent
	Total  int
}

type DecisionAuditReader interface {
	SearchDecisionAudit(context.Context, DecisionAuditQuery) (DecisionAuditPage, error)
}
