package authorization

import "time"

type ConsumerKey string
type CapabilityKey string
type ScopeType string
type RoleKey string
type AccessLayerKey string
type ConstraintKey string
type TriggerKey string
type PredicateKey string

const (
	AccessLayerVisitor       AccessLayerKey = "visitor"
	AccessLayerAuthenticated AccessLayerKey = "authenticated"
)

const (
	CapabilityManage              CapabilityKey = "authorization.manage"
	CapabilityAuditRead           CapabilityKey = "authorization.audit.read"
	CapabilityApplicationCreate   CapabilityKey = "authorization.application.create"
	CapabilityApplicationReadOwn  CapabilityKey = "authorization.application.read_own"
	CapabilityApplicationWithdraw CapabilityKey = "authorization.application.withdraw_own"
	CapabilityInvitationAccept    CapabilityKey = "authorization.invitation.accept"
)

// SubjectKind describes the identity proof presented to a consumer.
type SubjectKind string

const (
	SubjectAnonymous SubjectKind = "anonymous"
	SubjectGuest     SubjectKind = "guest"
	SubjectUser      SubjectKind = "user"
	SubjectService   SubjectKind = "service"
	SubjectGroup     SubjectKind = "group"
)

// SubjectRef identifies a caller or grant target. Anonymous has no ID.
type SubjectRef struct {
	Kind SubjectKind
	ID   string
}

// BindingClass limits which policy target may receive a capability.
type BindingClass string

const (
	BindingNormal              BindingClass = "normal"
	BindingProtectedOnly       BindingClass = "protected_only"
	BindingAccessLayerEligible BindingClass = "access_layer_eligible"
)

type RiskLevel string

const (
	RiskNormal RiskLevel = "normal"
	RiskHigh   RiskLevel = "high"
)

type AuditMode string

const (
	AuditDeniedAndHighRisk AuditMode = "denied_and_high_risk"
	AuditFull              AuditMode = "full"
)

// GrantSource explains how a role assignment was created.
type GrantSource string

const (
	GrantSourceAutomatic           GrantSource = "automatic"
	GrantSourceApplication         GrantSource = "application"
	GrantSourceInvitation          GrantSource = "invitation"
	GrantSourceDirect              GrantSource = "direct"
	GrantSourceGroup               GrantSource = "group"
	GrantSourceServiceProvisioning GrantSource = "service_provisioning"
	GrantSourceImportSync          GrantSource = "import_sync"
	GrantSourceBootstrap           GrantSource = "bootstrap"
	GrantSourceRecovery            GrantSource = "recovery"
	GrantSourceInitialClaim        GrantSource = "initial_claim"
)

// AssignmentPolicy limits how a non-protected role may be assigned.
type AssignmentPolicy struct {
	Sources     []GrantSource
	MaxDuration time.Duration
}
