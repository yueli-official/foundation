// Package repository defines the private state-transfer seam shared by the
// domain kernel and durable Adapters. It deliberately contains only storage
// primitives and cannot be imported outside the authorization module tree.
package repository

import "time"

type Subject struct {
	Kind string
	ID   string
}

type Scope struct {
	ID       string
	Type     string
	ParentID string
}

type PolicyRevision struct {
	Number      uint64
	Base        uint64
	State       string
	CreatedBy   Subject
	CreatedAt   time.Time
	ActivatedAt time.Time
}

type Role struct {
	ID           string
	Key          string
	DisplayName  string
	ScopeID      string
	Kind         string
	Status       string
	Protected    bool
	Capabilities []string
	Sources      []string
	MaxDuration  time.Duration
}

type Policy struct {
	Revision       PolicyRevision
	Roles          []Role
	AccessLayers   map[string][]string
	AutomaticRules map[string]bool
	TouchedScopes  []string
}

type Grant struct {
	ID        string
	Target    Subject
	RoleID    string
	RoleKey   string
	ScopeID   string
	Source    string
	ValidFrom time.Time
	ExpiresAt time.Time
	RevokedAt time.Time
}

type Group struct {
	ID          string
	ScopeID     string
	DisplayName string
	Members     []Subject
}

type Application struct {
	ID             string
	RequestGroupID string
	Subject        Subject
	RoleID         string
	RoleKey        string
	ScopeID        string
	Reason         string
	State          string
	GrantID        string
	CreatedAt      time.Time
	ReviewedAt     time.Time
	ReviewedBy     Subject
	ReviewReason   string
	IdempotencyKey string
}

type Invitation struct {
	ID          string
	Subject     Subject
	Email       string
	RoleID      string
	RoleKey     string
	ScopeID     string
	State       string
	InvitedBy   Subject
	AcceptedBy  Subject
	GrantID     string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	CompletedAt time.Time
	TokenDigest []byte
}

type ReconcileResult struct {
	Subject Subject
	Grants  []Grant
	Created int
}

type InboxEvent struct {
	ID     string
	Result ReconcileResult
}

type AuditEvent struct {
	ID             string
	Action         string
	Actor          Subject
	Subject        Subject
	RoleKey        string
	ScopeID        string
	PolicyRevision uint64
	CorrelationID  string
	OccurredAt     time.Time
}

type DecisionSource struct {
	Kind        string
	RoleID      string
	RoleKey     string
	AccessLayer string
	GrantID     string
	GroupID     string
}

type DecisionAuditEvent struct {
	DecisionID       string
	Subject          Subject
	Capability       string
	ScopeID          string
	ResourceType     string
	ResourceID       string
	ResourceRevision string
	Allowed          bool
	Reason           string
	Constraint       string
	PolicyRevision   uint64
	CorrelationID    string
	Sources          []DecisionSource
	OccurredAt       time.Time
}

type AutomaticRule struct {
	Key       string
	Trigger   string
	Predicate string
	RoleKey   string
	Enabled   bool
}

type Snapshot struct {
	RootScopeID         string
	ActivePolicy        uint64
	NextPolicy          uint64
	NextID              uint64
	CatalogCapabilities []string
	AutomaticRules      []AutomaticRule
	Scopes              []Scope
	Policies            []Policy
	Grants              []Grant
	Groups              []Group
	Applications        []Application
	Invitations         []Invitation
	Inbox               []InboxEvent
	Audit               []AuditEvent
	DecisionAudit       []DecisionAuditEvent
}
