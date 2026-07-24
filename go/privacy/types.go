package privacy

import "time"

type Revision uint64
type PurposeKey string
type NoticeKey string
type DataCategoryKey string
type SignalKey string
type RestrictionKey string
type RetentionRuleKey string
type RetentionTriggerKey string
type OwnerKey string
type DatasetKey string
type SubjectKind string
type ReasonCode string
type IdempotencyKey string
type ReceiptID string
type ChannelKey string
type RightsRequestID string
type OwnerTaskID string
type RetentionItemID string

type PurposeRef struct {
	Key      PurposeKey `json:"key"`
	Revision Revision   `json:"revision"`
}

type NoticeRef struct {
	Key      NoticeKey `json:"key"`
	Revision Revision  `json:"revision"`
}

type RetentionRuleRef struct {
	Key      RetentionRuleKey `json:"key"`
	Revision Revision         `json:"revision"`
}

type OwnerRef struct {
	Key      OwnerKey `json:"key"`
	Revision Revision `json:"revision"`
	Digest   string   `json:"digest"`
}

type SubjectRef struct {
	Owner OwnerKey    `json:"owner"`
	Kind  SubjectKind `json:"kind"`
	Value string      `json:"value"`
}

type SubjectContext struct {
	Current *SubjectRef  `json:"current,omitempty"`
	Aliases []SubjectRef `json:"aliases,omitempty"`
}

func SingleSubject(subject SubjectRef) SubjectContext {
	return SubjectContext{Current: &subject}
}

type ProcessingBasis string

const (
	BasisConsent            ProcessingBasis = "consent"
	BasisContract           ProcessingBasis = "contract"
	BasisLegalObligation    ProcessingBasis = "legal_obligation"
	BasisVitalInterests     ProcessingBasis = "vital_interests"
	BasisPublicTask         ProcessingBasis = "public_task"
	BasisLegitimateInterest ProcessingBasis = "legitimate_interests"
)

type DecisionKind string

const (
	DecisionAllow    DecisionKind = "allow"
	DecisionDeny     DecisionKind = "deny"
	DecisionRestrict DecisionKind = "restrict"
)

type ObservedSignal struct {
	Signal     SignalKey `json:"signal"`
	AssertedAt time.Time `json:"assertedAt"`
}

type DecisionInput struct {
	Subject SubjectContext   `json:"subject"`
	At      time.Time        `json:"at,omitempty"`
	Signals []ObservedSignal `json:"signals,omitempty"`
}

type ProcessingDecision struct {
	Kind          DecisionKind     `json:"kind"`
	Purpose       PurposeRef       `json:"purpose"`
	Basis         ProcessingBasis  `json:"basis"`
	Restrictions  []RestrictionKey `json:"restrictions,omitempty"`
	Reasons       []ReasonCode     `json:"reasons"`
	Evidence      []ReceiptID      `json:"evidence,omitempty"`
	CatalogDigest string           `json:"catalogDigest"`
	DecidedAt     time.Time        `json:"decidedAt"`
}

func (decision ProcessingDecision) Allows(handled ...RestrictionKey) bool {
	if decision.Kind == DecisionAllow {
		return true
	}
	if decision.Kind != DecisionRestrict {
		return false
	}
	set := make(map[RestrictionKey]struct{}, len(handled))
	for _, key := range handled {
		set[key] = struct{}{}
	}
	for _, key := range decision.Restrictions {
		if _, ok := set[key]; !ok {
			return false
		}
	}
	return true
}

type ConsentCommand struct {
	IdempotencyKey IdempotencyKey `json:"idempotencyKey"`
	Subject        SubjectRef     `json:"subject"`
	Notice         NoticeRef      `json:"notice"`
	Purposes       []PurposeRef   `json:"purposes"`
	OccurredAt     time.Time      `json:"occurredAt"`
	Channel        ChannelKey     `json:"channel"`
	EvidenceDigest string         `json:"evidenceDigest,omitempty"`
}

type ConsentReceipt struct {
	ID             ReceiptID    `json:"id"`
	Subject        SubjectRef   `json:"subject"`
	Notice         NoticeRef    `json:"notice"`
	Purposes       []PurposeRef `json:"purposes"`
	OccurredAt     time.Time    `json:"occurredAt"`
	RecordedAt     time.Time    `json:"recordedAt"`
	Channel        ChannelKey   `json:"channel"`
	EvidenceDigest string       `json:"evidenceDigest,omitempty"`
	Fingerprint    string       `json:"fingerprint"`
	Replay         bool         `json:"replay"`
}

type WithdrawalCommand struct {
	IdempotencyKey IdempotencyKey `json:"idempotencyKey"`
	Subject        SubjectRef     `json:"subject"`
	Purposes       []PurposeRef   `json:"purposes"`
	OccurredAt     time.Time      `json:"occurredAt"`
	Channel        ChannelKey     `json:"channel"`
	Reason         ReasonCode     `json:"reason,omitempty"`
}

type WithdrawalReceipt struct {
	ID          ReceiptID    `json:"id"`
	Subject     SubjectRef   `json:"subject"`
	Purposes    []PurposeRef `json:"purposes"`
	Supersedes  []ReceiptID  `json:"supersedes,omitempty"`
	OccurredAt  time.Time    `json:"occurredAt"`
	RecordedAt  time.Time    `json:"recordedAt"`
	Channel     ChannelKey   `json:"channel"`
	Reason      ReasonCode   `json:"reason,omitempty"`
	Fingerprint string       `json:"fingerprint"`
	Replay      bool         `json:"replay"`
}

type SignalCommand struct {
	IdempotencyKey IdempotencyKey `json:"idempotencyKey"`
	Subject        SubjectRef     `json:"subject"`
	Signal         SignalKey      `json:"signal"`
	AssertedAt     time.Time      `json:"assertedAt"`
	ExpiresAt      *time.Time     `json:"expiresAt,omitempty"`
	Channel        ChannelKey     `json:"channel"`
	EvidenceDigest string         `json:"evidenceDigest,omitempty"`
}

type SignalReceipt struct {
	ID          ReceiptID  `json:"id"`
	Subject     SubjectRef `json:"subject"`
	Signal      SignalKey  `json:"signal"`
	AssertedAt  time.Time  `json:"assertedAt"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	RecordedAt  time.Time  `json:"recordedAt"`
	Channel     ChannelKey `json:"channel"`
	Fingerprint string     `json:"fingerprint"`
	Replay      bool       `json:"replay"`
}

type CalendarPeriod struct {
	Years  int `json:"years,omitempty"`
	Months int `json:"months,omitempty"`
	Days   int `json:"days,omitempty"`
}

func (period CalendarPeriod) Add(value time.Time) time.Time {
	return value.AddDate(period.Years, period.Months, period.Days)
}

type RecordRef struct {
	Dataset DatasetKey `json:"dataset"`
	Value   string     `json:"value"`
}

type RetentionCommand struct {
	IdempotencyKey IdempotencyKey   `json:"idempotencyKey"`
	Record         RecordRef        `json:"record"`
	Rule           RetentionRuleRef `json:"rule"`
	TriggeredAt    time.Time        `json:"triggeredAt"`
}

type RetentionState string

const (
	RetentionTracked   RetentionState = "tracked"
	RetentionDue       RetentionState = "due"
	RetentionRetained  RetentionState = "retained"
	RetentionCompleted RetentionState = "completed"
)

type OwnerDisposition string

const (
	DispositionExported   OwnerDisposition = "exported"
	DispositionRectified  OwnerDisposition = "rectified"
	DispositionRestricted OwnerDisposition = "restricted"
	DispositionDeleted    OwnerDisposition = "deleted"
	DispositionAnonymized OwnerDisposition = "anonymized"
	DispositionRetained   OwnerDisposition = "retained"
	DispositionNotFound   OwnerDisposition = "not_found"
	DispositionRefused    OwnerDisposition = "refused"
)

type RetentionItem struct {
	ID          RetentionItemID  `json:"id"`
	Record      RecordRef        `json:"record"`
	Rule        RetentionRuleRef `json:"rule"`
	TriggeredAt time.Time        `json:"triggeredAt"`
	ReviewAt    time.Time        `json:"reviewAt"`
	State       RetentionState   `json:"state"`
	LastOutcome OwnerDisposition `json:"lastOutcome,omitempty"`
	Reason      ReasonCode       `json:"reason,omitempty"`
	Fingerprint string           `json:"fingerprint"`
	Replay      bool             `json:"replay"`
}

type RetentionReviewCommand struct {
	IdempotencyKey IdempotencyKey   `json:"idempotencyKey"`
	ItemID         RetentionItemID  `json:"itemId"`
	Outcome        OwnerDisposition `json:"outcome"`
	Reason         ReasonCode       `json:"reason,omitempty"`
	ReviewAfter    *time.Time       `json:"reviewAfter,omitempty"`
	HoldRef        string           `json:"holdRef,omitempty"`
}

type RetentionDueQuery struct {
	At     time.Time `json:"at"`
	Limit  int       `json:"limit,omitempty"`
	Cursor string    `json:"cursor,omitempty"`
}

type RetentionPage struct {
	Items      []RetentionItem `json:"items"`
	NextCursor string          `json:"nextCursor,omitempty"`
}

type RightsOperation string

const (
	RightAccess        RightsOperation = "access"
	RightPortability   RightsOperation = "portability"
	RightRectification RightsOperation = "rectification"
	RightErasure       RightsOperation = "erasure"
	RightRestriction   RightsOperation = "restriction"
	RightObjection     RightsOperation = "objection"
	RetentionReview    RightsOperation = "retention_review"
)

type VerificationEvidence struct {
	VerifiedAt      time.Time `json:"verifiedAt"`
	Method          string    `json:"method"`
	Assurance       string    `json:"assurance"`
	VerificationRef string    `json:"verificationRef"`
}

type OpenRightsRequest struct {
	IdempotencyKey IdempotencyKey       `json:"idempotencyKey"`
	Subject        SubjectContext       `json:"subject"`
	Operation      RightsOperation      `json:"operation"`
	Verification   VerificationEvidence `json:"verification"`
	RequestedAt    time.Time            `json:"requestedAt"`
	Channel        ChannelKey           `json:"channel"`
}

type RequestPhase string

const (
	RequestOpen     RequestPhase = "open"
	RequestActive   RequestPhase = "active"
	RequestPartial  RequestPhase = "partial"
	RequestComplete RequestPhase = "complete"
)

type TaskPhase string

const (
	TaskPending  TaskPhase = "pending"
	TaskInFlight TaskPhase = "in_flight"
	TaskWaiting  TaskPhase = "waiting"
	TaskTerminal TaskPhase = "terminal"
)

type OwnerTaskView struct {
	ID            OwnerTaskID   `json:"id"`
	Owner         OwnerRef      `json:"owner"`
	Phase         TaskPhase     `json:"phase"`
	Attempt       uint32        `json:"attempt"`
	NextAttemptAt *time.Time    `json:"nextAttemptAt,omitempty"`
	Receipt       *OwnerReceipt `json:"receipt,omitempty"`
}

type RightsSummary struct {
	Performed int `json:"performed"`
	Retained  int `json:"retained"`
	Refused   int `json:"refused"`
	NoRecords int `json:"noRecords"`
	Pending   int `json:"pending"`
}

type RightsRequestView struct {
	ID          RightsRequestID `json:"id"`
	Operation   RightsOperation `json:"operation"`
	Phase       RequestPhase    `json:"phase"`
	Overdue     bool            `json:"overdue"`
	Deadline    time.Time       `json:"deadline"`
	RequestedAt time.Time       `json:"requestedAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	Tasks       []OwnerTaskView `json:"tasks"`
	Summary     RightsSummary   `json:"summary"`
	Fingerprint string          `json:"fingerprint"`
	Replay      bool            `json:"replay"`
}

type DriveBudget struct {
	MaxOwnerAttempts int           `json:"maxOwnerAttempts,omitempty"`
	MaxDuration      time.Duration `json:"maxDuration,omitempty"`
}

type DriveRightsRequest struct {
	Request RightsRequestID `json:"request"`
	Budget  DriveBudget     `json:"budget"`
}

type DriveResult struct {
	View          RightsRequestView `json:"view"`
	NextAttemptAt *time.Time        `json:"nextAttemptAt,omitempty"`
}

type OwnerCommand struct {
	ProtocolVersion uint64          `json:"protocolVersion"`
	RequestID       RightsRequestID `json:"requestId"`
	TaskID          OwnerTaskID     `json:"taskId"`
	Owner           OwnerRef        `json:"owner"`
	Operation       RightsOperation `json:"operation"`
	Subject         SubjectContext  `json:"subject"`
	Datasets        []DatasetKey    `json:"datasets"`
	RequestedAt     time.Time       `json:"requestedAt"`
	Deadline        time.Time       `json:"deadline"`
	Fingerprint     string          `json:"fingerprint"`
}

type OwnerInstruction struct {
	Command OwnerCommand `json:"command"`
	Attempt uint32       `json:"attempt"`
}

type ArtifactRef struct {
	Provider  string    `json:"provider"`
	Key       string    `json:"key"`
	MediaType string    `json:"mediaType"`
	Digest    string    `json:"digest"`
	SizeBytes int64     `json:"sizeBytes"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type DatasetOutcome struct {
	Dataset     DatasetKey       `json:"dataset"`
	Disposition OwnerDisposition `json:"disposition"`
	Count       int64            `json:"count"`
	Reason      ReasonCode       `json:"reason,omitempty"`
	ReviewAfter *time.Time       `json:"reviewAfter,omitempty"`
	Artifacts   []ArtifactRef    `json:"artifacts,omitempty"`
}

type OwnerOutcome struct {
	Terminal   bool             `json:"terminal"`
	Results    []DatasetOutcome `json:"results"`
	RetryAfter *time.Time       `json:"retryAfter,omitempty"`
}

type OwnerReceipt struct {
	ProtocolVersion    uint64           `json:"protocolVersion"`
	ID                 string           `json:"id"`
	RequestID          RightsRequestID  `json:"requestId"`
	TaskID             OwnerTaskID      `json:"taskId"`
	Owner              OwnerRef         `json:"owner"`
	CommandFingerprint string           `json:"commandFingerprint"`
	Sequence           uint64           `json:"sequence"`
	Terminal           bool             `json:"terminal"`
	Results            []DatasetOutcome `json:"results"`
	RetryAfter         *time.Time       `json:"retryAfter,omitempty"`
	RecordedAt         time.Time        `json:"recordedAt"`
	Fingerprint        string           `json:"fingerprint"`
	Replay             bool             `json:"replay"`
}
