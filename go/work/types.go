package work

import (
	"encoding/json"
	"time"
)

type JobID string
type Kind string
type Queue string
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusRetrying  Status = "retrying"
	StatusPaused    Status = "paused"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Request struct {
	Kind           Kind            `json:"kind"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	RunAt          time.Time       `json:"runAt,omitempty"`
	Priority       int             `json:"priority,omitempty"`
	MaxAttempts    int             `json:"maxAttempts,omitempty"`
	IdempotencyKey string          `json:"idempotencyKey,omitempty"`
}

type PreparedRequest struct {
	Kind           Kind
	Queue          Queue
	Payload        json.RawMessage
	Metadata       json.RawMessage
	RunAt          time.Time
	Priority       int
	MaxAttempts    int
	IdempotencyKey string
	Fingerprint    [32]byte
}

type Job struct {
	ID             JobID           `json:"id"`
	Kind           Kind            `json:"kind"`
	Queue          Queue           `json:"queue"`
	Status         Status          `json:"status"`
	Payload        json.RawMessage `json:"payload"`
	Metadata       json.RawMessage `json:"metadata"`
	Progress       json.RawMessage `json:"progress,omitempty"`
	Result         json.RawMessage `json:"result,omitempty"`
	ResultSummary  string          `json:"resultSummary,omitempty"`
	LastError      string          `json:"lastError,omitempty"`
	Attempt        int             `json:"attempt"`
	MaxAttempts    int             `json:"maxAttempts"`
	Priority       int             `json:"priority"`
	RunAt          time.Time       `json:"runAt"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	StartedAt      *time.Time      `json:"startedAt,omitempty"`
	FinishedAt     *time.Time      `json:"finishedAt,omitempty"`
	CancelledAt    *time.Time      `json:"cancelledAt,omitempty"`
	ReplayOf       JobID           `json:"replayOf,omitempty"`
	IdempotencyKey string          `json:"idempotencyKey,omitempty"`
	LeaseOwner     string          `json:"leaseOwner,omitempty"`
	LeaseExpiresAt *time.Time      `json:"leaseExpiresAt,omitempty"`
}

type EnqueueResult struct {
	Job    Job  `json:"job"`
	Replay bool `json:"replay"`
}

type AttemptOutcome string

const (
	AttemptRunning      AttemptOutcome = "running"
	AttemptSucceeded    AttemptOutcome = "succeeded"
	AttemptRetrying     AttemptOutcome = "retrying"
	AttemptFailed       AttemptOutcome = "failed"
	AttemptCancelled    AttemptOutcome = "cancelled"
	AttemptPaused       AttemptOutcome = "paused"
	AttemptLeaseExpired AttemptOutcome = "lease_expired"
)

type Attempt struct {
	JobID      JobID          `json:"jobId"`
	Number     int            `json:"number"`
	WorkerID   string         `json:"workerId"`
	Outcome    AttemptOutcome `json:"outcome"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `json:"startedAt"`
	FinishedAt *time.Time     `json:"finishedAt,omitempty"`
}

type Result struct {
	Summary string          `json:"summary,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type ListQuery struct {
	Statuses []Status
	Kinds    []Kind
	Queues   []Queue
	Limit    int
	Offset   int
}

type ReplayRequest struct {
	RunAt          time.Time
	Priority       *int
	MaxAttempts    int
	IdempotencyKey string
}

type Stats struct {
	ByStatus    map[Status]int64 `json:"byStatus"`
	Due         int64            `json:"due"`
	Running     int64            `json:"running"`
	OldestDueAt *time.Time       `json:"oldestDueAt,omitempty"`
}

type PruneResult struct {
	JobsRemoved     int64 `json:"jobsRemoved"`
	AttemptsRemoved int64 `json:"attemptsRemoved"`
}

type Lease struct {
	Job       Job
	Token     string
	ExpiresAt time.Time
}

type ClaimRequest struct {
	Queue         Queue
	WorkerID      string
	LeaseDuration time.Duration
}

type ClaimResult struct {
	Lease Lease
	Found bool
}

type RetryCommand struct {
	Lease Lease
	Error string
	RunAt time.Time
}

type FailCommand struct {
	Lease Lease
	Error string
}

type CompleteCommand struct {
	Lease  Lease
	Result Result
}
