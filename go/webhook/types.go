package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

type EventType string
type EventID string
type EndpointID string
type SubscriptionID string
type DeliveryID string
type AttemptID string
type InboundSource string
type SecretRef string
type SecretRevision string

type EndpointState string

const (
	EndpointActive   EndpointState = "active"
	EndpointPaused   EndpointState = "paused"
	EndpointDisabled EndpointState = "disabled"
	EndpointRevoked  EndpointState = "revoked"
)

type DeliveryState string

const (
	DeliveryPending    DeliveryState = "pending"
	DeliveryDelivering DeliveryState = "delivering"
	DeliveryRetrying   DeliveryState = "retrying"
	DeliveryDelivered  DeliveryState = "delivered"
	DeliveryFailed     DeliveryState = "failed"
	DeliveryPaused     DeliveryState = "paused"
	DeliveryCancelled  DeliveryState = "cancelled"
)

type AttemptOutcome string

const (
	AttemptSucceeded AttemptOutcome = "succeeded"
	AttemptRetryable AttemptOutcome = "retryable"
	AttemptPermanent AttemptOutcome = "permanent"
	AttemptUnknown   AttemptOutcome = "unknown"
)

type EventCommand struct {
	Type           EventType
	Subject        string
	Data           json.RawMessage
	OccurredAt     time.Time
	IdempotencyKey string
	TraceParent    string
}

type EventReceipt struct {
	EventID     EventID   `json:"eventId"`
	Deliveries  int       `json:"deliveries"`
	Duplicate   bool      `json:"duplicate"`
	BodyDigest  string    `json:"bodyDigest"`
	PublishedAt time.Time `json:"publishedAt"`
}

type Endpoint struct {
	ID          EndpointID    `json:"id"`
	Revision    uint64        `json:"revision"`
	URL         string        `json:"url"`
	Description string        `json:"description,omitempty"`
	State       EndpointState `json:"state"`
	ETag        string        `json:"etag"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}

type EndpointCredential struct {
	Endpoint Endpoint `json:"endpoint"`
	Secret   string   `json:"secret,omitempty"`
}

type PutEndpointCommand struct {
	ID           EndpointID
	URL          string
	Description  string
	ExpectedETag string
}

type SetEndpointStateCommand struct {
	EndpointID   EndpointID
	State        EndpointState
	Reason       string
	ExpectedETag string
}

type RotateSecretCommand struct {
	EndpointID EndpointID
	Overlap    time.Duration
}

type Subscription struct {
	ID          SubscriptionID `json:"id"`
	Revision    uint64         `json:"revision"`
	EndpointID  EndpointID     `json:"endpointId"`
	EventTypes  []EventType    `json:"eventTypes"`
	Enabled     bool           `json:"enabled"`
	Description string         `json:"description,omitempty"`
	ETag        string         `json:"etag"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

type PutSubscriptionCommand struct {
	ID           SubscriptionID
	EndpointID   EndpointID
	EventTypes   []EventType
	Enabled      bool
	Description  string
	ExpectedETag string
}

type DeliveryView struct {
	ID                   DeliveryID     `json:"id"`
	EventID              EventID        `json:"eventId"`
	EndpointID           EndpointID     `json:"endpointId"`
	EndpointRevision     uint64         `json:"endpointRevision"`
	SubscriptionID       SubscriptionID `json:"subscriptionId"`
	SubscriptionRevision uint64         `json:"subscriptionRevision"`
	State                DeliveryState  `json:"state"`
	AttemptCount         int            `json:"attemptCount"`
	NextAttemptAt        time.Time      `json:"nextAttemptAt,omitempty"`
	LastErrorCode        string         `json:"lastErrorCode,omitempty"`
	ReplayOf             DeliveryID     `json:"replayOf,omitempty"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
}

type EventView struct {
	ID             EventID         `json:"id"`
	Type           EventType       `json:"type"`
	Subject        string          `json:"subject,omitempty"`
	Body           json.RawMessage `json:"body,omitempty"`
	BodyDigest     string          `json:"bodyDigest"`
	IdempotencyKey string          `json:"idempotencyKey"`
	OccurredAt     time.Time       `json:"occurredAt"`
	PublishedAt    time.Time       `json:"publishedAt"`
}

type AttemptView struct {
	ID             AttemptID      `json:"id"`
	DeliveryID     DeliveryID     `json:"deliveryId"`
	Number         int            `json:"number"`
	Outcome        AttemptOutcome `json:"outcome"`
	StatusCode     int            `json:"statusCode,omitempty"`
	ErrorCode      string         `json:"errorCode,omitempty"`
	RequestDigest  string         `json:"requestDigest"`
	ResponseDigest string         `json:"responseDigest,omitempty"`
	SecretRevision SecretRevision `json:"secretRevision,omitempty"`
	StartedAt      time.Time      `json:"startedAt"`
	FinishedAt     *time.Time     `json:"finishedAt,omitempty"`
}

type DeliveryQuery struct {
	EndpointID EndpointID
	EventType  EventType
	States     []DeliveryState
	Since      time.Time
	Until      time.Time
	Limit      int
	After      string
}

type DeliveryPage struct {
	Deliveries []DeliveryView `json:"deliveries"`
	Next       string         `json:"next,omitempty"`
}

type ReplayCommand struct {
	DeliveryID     DeliveryID
	Reason         string
	IdempotencyKey string
}

type ReplayReceipt struct {
	Delivery  DeliveryView `json:"delivery"`
	Duplicate bool         `json:"duplicate"`
}

type MetricsSnapshot struct {
	ByState     map[DeliveryState]int64 `json:"byState"`
	Due         int64                   `json:"due"`
	OldestDueAt *time.Time              `json:"oldestDueAt,omitempty"`
}

type IncomingMessage struct {
	Headers    http.Header
	Body       []byte
	ReceivedAt time.Time
}

// VerifiedInbound can only be produced by a Verifier from this package.
type VerifiedInbound struct {
	source         InboundSource
	eventID        string
	eventType      EventType
	occurredAt     time.Time
	attemptedAt    time.Time
	receivedAt     time.Time
	body           []byte
	bodyDigest     string
	secretRevision SecretRevision
	catalogDigest  string
}

func (value VerifiedInbound) Source() InboundSource          { return value.source }
func (value VerifiedInbound) EventID() string                { return value.eventID }
func (value VerifiedInbound) EventType() EventType           { return value.eventType }
func (value VerifiedInbound) OccurredAt() time.Time          { return value.occurredAt }
func (value VerifiedInbound) AttemptedAt() time.Time         { return value.attemptedAt }
func (value VerifiedInbound) ReceivedAt() time.Time          { return value.receivedAt }
func (value VerifiedInbound) Body() []byte                   { return append([]byte(nil), value.body...) }
func (value VerifiedInbound) BodyDigest() string             { return value.bodyDigest }
func (value VerifiedInbound) SecretRevision() SecretRevision { return value.secretRevision }

type InboundReceipt struct {
	ReceiptID   string         `json:"receiptId"`
	Source      InboundSource  `json:"source"`
	EventID     string         `json:"eventId"`
	BodyDigest  string         `json:"bodyDigest"`
	KeyRevision SecretRevision `json:"keyRevision"`
	FirstSeen   bool           `json:"firstSeen"`
	ReceivedAt  time.Time      `json:"receivedAt"`
	AcceptedAt  time.Time      `json:"acceptedAt"`
}

type Publisher interface {
	Publish(context.Context, EventCommand) (EventReceipt, error)
}

type ControlPlane interface {
	PutEndpoint(context.Context, PutEndpointCommand) (EndpointCredential, error)
	PutSubscription(context.Context, PutSubscriptionCommand) (Subscription, error)
	SetEndpointState(context.Context, SetEndpointStateCommand) (Endpoint, error)
	RotateSecret(context.Context, RotateSecretCommand) (EndpointCredential, error)
}

type Operations interface {
	Event(context.Context, EventID) (EventView, error)
	Delivery(context.Context, DeliveryID) (DeliveryView, error)
	ListDeliveries(context.Context, DeliveryQuery) (DeliveryPage, error)
	Attempts(context.Context, DeliveryID) ([]AttemptView, error)
	Replay(context.Context, ReplayCommand) (ReplayReceipt, error)
	Snapshot(context.Context) (MetricsSnapshot, error)
}

type Verifier interface {
	Verify(context.Context, InboundSource, IncomingMessage) (VerifiedInbound, error)
}

type ReceiptLedger interface {
	Accept(context.Context, VerifiedInbound) (InboundReceipt, error)
}

type Runtime interface {
	Publisher
	ControlPlane
	Operations
	Verifier
	ReceiptLedger
}

type DeliveryWork struct {
	DeliveryID DeliveryID `json:"deliveryId"`
	RunAt      time.Time  `json:"runAt"`
	Key        string     `json:"idempotencyKey"`
}

type Scheduler interface {
	Enqueue(context.Context, DeliveryWork) error
}

type TransactionalScheduler interface {
	Scheduler
	EnqueueTx(context.Context, *sql.Tx, DeliveryWork) error
}

type SecretMaterial struct {
	Revision  SecretRevision
	Value     []byte
	NotBefore time.Time
	NotAfter  *time.Time
}

type SecretSet struct {
	Primary  SecretMaterial
	Previous []SecretMaterial
}

type SecretStore interface {
	Create(context.Context, SecretRef, SecretMaterial) error
	Resolve(context.Context, SecretRef, time.Time) (SecretSet, error)
	Rotate(context.Context, SecretRef, SecretMaterial, time.Time) error
	Delete(context.Context, SecretRef, SecretRevision) error
}

type AttemptPlan struct {
	AttemptID       AttemptID
	DeliveryID      DeliveryID
	EventID         EventID
	EventType       EventType
	EndpointID      EndpointID
	URL             string
	Secret          SecretRef
	Body            []byte
	BodyDigest      string
	Number          int
	DeliveryCreated time.Time
}

type AttemptCompletion struct {
	Plan            AttemptPlan
	Outcome         AttemptOutcome
	StatusCode      int
	ErrorCode       string
	ResponseDigest  string
	SecretRevision  SecretRevision
	FinishedAt      time.Time
	NextAttemptAt   time.Time
	DisableEndpoint bool
}

type DeliveryBackend interface {
	BeginAttempt(context.Context, DeliveryID) (AttemptPlan, error)
	CompleteAttempt(context.Context, AttemptCompletion) (DeliveryView, error)
}
