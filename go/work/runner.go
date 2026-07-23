package work

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Progress interface {
	Save(context.Context, json.RawMessage) error
}

type Handler interface {
	Handle(context.Context, Job, Progress) (Result, error)
}

type HandlerFunc func(context.Context, Job, Progress) (Result, error)

func (function HandlerFunc) Handle(ctx context.Context, job Job, progress Progress) (Result, error) {
	return function(ctx, job, progress)
}

type RunnerOptions struct {
	WorkerID          string
	PollInterval      time.Duration
	ScheduleInterval  time.Duration
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	Clock             func() time.Time
	OnError           func(error)
}

type Runner struct {
	catalog  *Catalog
	backend  Backend
	handlers map[Kind]Handler
	options  RunnerOptions
}

func NewRunner(catalog *Catalog, backend Backend, handlers map[Kind]Handler, options RunnerOptions) (*Runner, error) {
	if catalog == nil {
		return nil, invalid("catalog", "is required")
	}
	if backend == nil {
		return nil, invalid("backend", "is required")
	}
	workerID := strings.TrimSpace(options.WorkerID)
	if workerID == "" {
		return nil, invalid("worker_id", "is required")
	}
	if len(workerID) > 200 {
		return nil, invalid("worker_id", "exceeds 200 bytes")
	}
	if options.PollInterval == 0 {
		options.PollInterval = time.Second
	}
	if options.ScheduleInterval == 0 {
		options.ScheduleInterval = 30 * time.Second
	}
	if options.LeaseDuration == 0 {
		options.LeaseDuration = time.Minute
	}
	if options.HeartbeatInterval == 0 {
		options.HeartbeatInterval = options.LeaseDuration / 3
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.PollInterval <= 0 || options.ScheduleInterval <= 0 || options.HeartbeatInterval <= 0 {
		return nil, invalid("runner", "poll, schedule and heartbeat intervals must be positive")
	}
	if err := catalog.validateLease(options.LeaseDuration); err != nil {
		return nil, err
	}
	if options.HeartbeatInterval*2 >= options.LeaseDuration {
		return nil, invalid("heartbeat_interval", "must be less than half the lease duration")
	}
	copied := make(map[Kind]Handler, len(handlers))
	for kind, handler := range handlers {
		if _, ok := catalog.kinds[kind]; !ok {
			return nil, invalid("handlers", "%q is not a registered kind", kind)
		}
		if handler == nil {
			return nil, invalid("handlers", "%q is nil", kind)
		}
		copied[kind] = handler
	}
	options.WorkerID = workerID
	return &Runner{catalog: catalog, backend: backend, handlers: copied, options: options}, nil
}

// Run serves every compiled queue with its configured concurrency until ctx is
// cancelled. Shutdown waits for handlers to return; their contexts are
// cancelled immediately and uncommitted leases are later reclaimed.
func (runner *Runner) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var wait sync.WaitGroup
	for _, queue := range runner.catalog.Queues() {
		for index := range queue.Concurrency {
			wait.Add(1)
			go func(queue Queue, index int) {
				defer wait.Done()
				runner.workerLoop(ctx, queue, fmt.Sprintf("%s/%s/%d", runner.options.WorkerID, queue, index+1))
			}(queue.Key, index)
		}
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		runner.scheduleLoop(ctx)
	}()
	<-ctx.Done()
	wait.Wait()
	return ctx.Err()
}

func (runner *Runner) workerLoop(ctx context.Context, queue Queue, workerID string) {
	for {
		worked, err := runner.RunOnce(ctx, queue, workerID)
		if err != nil && !errors.Is(err, context.Canceled) {
			runner.report(err)
		}
		if ctx.Err() != nil {
			return
		}
		if worked {
			continue
		}
		if !waitContext(ctx, runner.options.PollInterval) {
			return
		}
	}
}

func (runner *Runner) scheduleLoop(ctx context.Context) {
	for {
		if _, err := runner.backend.MaterializeSchedules(ctx); err != nil && !errors.Is(err, context.Canceled) {
			runner.report(err)
		}
		if !waitContext(ctx, runner.options.ScheduleInterval) {
			return
		}
	}
}

// RunOnce claims and executes at most one job. It is useful for deterministic
// tests and manually driven worker processes.
func (runner *Runner) RunOnce(ctx context.Context, queue Queue, workerID string) (bool, error) {
	claim, err := runner.backend.Claim(ctx, ClaimRequest{
		Queue: queue, WorkerID: workerID, LeaseDuration: runner.options.LeaseDuration,
	})
	if err != nil {
		return false, err
	}
	if !claim.Found {
		return false, nil
	}
	return true, runner.execute(ctx, claim.Lease)
}

func (runner *Runner) execute(parent context.Context, lease Lease) error {
	handler, ok := runner.handlers[lease.Job.Kind]
	if !ok {
		_, err := runner.backend.Fail(parent, FailCommand{
			Lease: lease, Error: "no handler is registered for job kind",
		})
		return err
	}
	handlerContext := parent
	cancel := func() {}
	if kind, exists := runner.catalog.Kind(lease.Job.Kind); exists && kind.Timeout > 0 {
		handlerContext, cancel = context.WithTimeout(parent, kind.Timeout)
	} else {
		handlerContext, cancel = context.WithCancel(parent)
	}
	defer cancel()

	heartbeatStop := make(chan struct{})
	heartbeatDone := make(chan struct{})
	leaseState := &runnerLease{lease: lease}
	go runner.heartbeatLoop(handlerContext, cancel, leaseState, heartbeatStop, heartbeatDone)

	result, handlerErr := handler.Handle(handlerContext, cloneJob(lease.Job), progressReporter{
		backend: runner.backend, lease: leaseState,
	})
	close(heartbeatStop)
	<-heartbeatDone
	current := leaseState.current()

	if parent.Err() != nil {
		return parent.Err()
	}
	if leaseState.lost() {
		return leaseLost("ownership changed while handler was running")
	}
	if handlerErr == nil {
		_, err := runner.backend.Complete(parent, CompleteCommand{Lease: current, Result: result})
		return err
	}
	permanent, retryAfter := classifyFailure(handlerErr)
	message := handlerErr.Error()
	if permanent || current.Job.Attempt >= current.Job.MaxAttempts {
		_, err := runner.backend.Fail(parent, FailCommand{Lease: current, Error: message})
		return err
	}
	if retryAfter <= 0 {
		retryAfter = runner.catalog.Backoff(current.Job.ID, current.Job.Attempt)
	}
	_, err := runner.backend.Retry(parent, RetryCommand{
		Lease: current, Error: message, RunAt: runner.options.Clock().UTC().Add(retryAfter),
	})
	return err
}

type runnerLease struct {
	mu      sync.RWMutex
	lease   Lease
	wasLost bool
}

func (state *runnerLease) current() Lease {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.lease
}

func (state *runnerLease) update(lease Lease) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.lease = lease
}

func (state *runnerLease) markLost() {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.wasLost = true
}

func (state *runnerLease) lost() bool {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.wasLost
}

func (runner *Runner) heartbeatLoop(
	ctx context.Context,
	cancel context.CancelFunc,
	state *runnerLease,
	stop <-chan struct{},
	done chan<- struct{},
) {
	defer close(done)
	ticker := time.NewTicker(runner.options.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := state.current()
			updated, err := runner.backend.Heartbeat(ctx, current, runner.options.LeaseDuration)
			if err != nil {
				if IsKind(err, ErrorLeaseLost) {
					state.markLost()
					cancel()
					return
				}
				runner.report(err)
				continue
			}
			state.update(updated)
		}
	}
}

type progressReporter struct {
	backend Backend
	lease   *runnerLease
}

func (reporter progressReporter) Save(ctx context.Context, value json.RawMessage) error {
	if reporter.lease.lost() {
		return leaseLost("ownership changed")
	}
	return reporter.backend.ReportProgress(ctx, reporter.lease.current(), value)
}

func (runner *Runner) report(err error) {
	if err != nil && runner.options.OnError != nil {
		runner.options.OnError(err)
	}
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
