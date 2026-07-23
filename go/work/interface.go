package work

import (
	"context"
	"encoding/json"
	"time"
)

type Enqueuer interface {
	Enqueue(context.Context, Request) (EnqueueResult, error)
}

type Reader interface {
	Get(context.Context, JobID) (Job, error)
	List(context.Context, ListQuery) ([]Job, error)
	Attempts(context.Context, JobID) ([]Attempt, error)
	Stats(context.Context) (Stats, error)
}

type Manager interface {
	Pause(context.Context, JobID) (Job, error)
	Resume(context.Context, JobID) (Job, error)
	Cancel(context.Context, JobID) (Job, error)
	Replay(context.Context, JobID, ReplayRequest) (EnqueueResult, error)
	Prune(context.Context, time.Time) (PruneResult, error)
}

type Module interface {
	Enqueuer
	Reader
	Manager
}

// Backend is the Adapter-author seam used by Runner. Product code normally
// depends on Module instead.
type Backend interface {
	Module
	Claim(context.Context, ClaimRequest) (ClaimResult, error)
	Heartbeat(context.Context, Lease, time.Duration) (Lease, error)
	ReportProgress(context.Context, Lease, json.RawMessage) error
	Complete(context.Context, CompleteCommand) (Job, error)
	Retry(context.Context, RetryCommand) (Job, error)
	Fail(context.Context, FailCommand) (Job, error)
	MaterializeSchedules(context.Context) (int, error)
}
