package audit

import "time"

type EventID string
type Sequence uint64
type ActionName string
type RetentionClass string
type EvidenceKey string
type ReasonCode string
type Cursor string
type Digest string

type Action struct {
	Name    ActionName `json:"name"`
	Version uint16     `json:"version"`
}

type ActorKind string

const (
	ActorUser      ActorKind = "user"
	ActorGuest     ActorKind = "guest"
	ActorService   ActorKind = "service"
	ActorSystem    ActorKind = "system"
	ActorAnonymous ActorKind = "anonymous"
)

type Actor struct {
	Kind ActorKind `json:"kind"`
	ID   string    `json:"id,omitempty"`
}

type Target struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type OutcomeKind string

const (
	OutcomeSucceeded OutcomeKind = "succeeded"
	OutcomeDenied    OutcomeKind = "denied"
	OutcomeFailed    OutcomeKind = "failed"
)

type Outcome struct {
	Kind   OutcomeKind `json:"kind"`
	Reason ReasonCode  `json:"reason,omitempty"`
}

type Correlation struct {
	RequestID   string `json:"requestId,omitempty"`
	TraceID     string `json:"traceId,omitempty"`
	SpanID      string `json:"spanId,omitempty"`
	CausationID string `json:"causationId,omitempty"`
	CommandID   string `json:"commandId,omitempty"`
	BatchID     string `json:"batchId,omitempty"`
}

type Source struct {
	Service  string `json:"service"`
	Module   string `json:"module,omitempty"`
	Instance string `json:"instance"`
	Version  string `json:"version,omitempty"`
}

type Event struct {
	ID               EventID         `json:"id"`
	EnvelopeVersion  uint16          `json:"envelopeVersion"`
	Sequence         Sequence        `json:"sequence"`
	Source           Source          `json:"source"`
	Action           Action          `json:"action"`
	Actor            Actor           `json:"actor"`
	Target           Target          `json:"target"`
	Outcome          Outcome         `json:"outcome"`
	Correlation      Correlation     `json:"correlation,omitempty"`
	Evidence         []EvidenceField `json:"evidence,omitempty"`
	RetentionClass   RetentionClass  `json:"retentionClass"`
	DefinitionDigest Digest          `json:"definitionDigest"`
	OccurredAt       time.Time       `json:"occurredAt"`
	RecordedAt       time.Time       `json:"recordedAt"`
	PreviousDigest   Digest          `json:"previousDigest,omitempty"`
	Digest           Digest          `json:"digest"`
}

type Clock interface {
	Now() time.Time
}

type ClockFunc func() time.Time

func (f ClockFunc) Now() time.Time { return f() }
