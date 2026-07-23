// Package worktest provides the public Adapter conformance suite.
package worktest

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/work"
)

type Clock struct {
	mu  sync.RWMutex
	now time.Time
}

func NewClock(now time.Time) *Clock {
	return &Clock{now: now}
}

func (clock *Clock) Now() time.Time {
	clock.mu.RLock()
	defer clock.mu.RUnlock()
	return clock.now
}

func (clock *Clock) Set(value time.Time) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = value
}

func (clock *Clock) Add(value time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(value)
}

type Factory func(*testing.T, *work.Catalog, *Clock) work.Backend

func Run(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("idempotent enqueue and conflict", func(t *testing.T) {
		backend, _, _ := newBackend(t, factory)
		ctx := context.Background()
		request := work.Request{
			Kind: "mail.send", Payload: json.RawMessage(`{"message":"a"}`),
			IdempotencyKey: "message:000000000001",
		}
		first, err := backend.Enqueue(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if first.Replay || first.Job.Status != work.StatusQueued || first.Job.Queue != "delivery" {
			t.Fatalf("unexpected first enqueue: %+v", first)
		}
		replay, err := backend.Enqueue(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if !replay.Replay || replay.Job.ID != first.Job.ID {
			t.Fatalf("unexpected replay: %+v", replay)
		}
		request.Payload = json.RawMessage(`{"message":"other"}`)
		if _, err := backend.Enqueue(ctx, request); !work.IsKind(err, work.ErrorConflict) {
			t.Fatalf("changed idempotency request must conflict, got %v", err)
		}
	})

	t.Run("claim ordering and exclusive lease", func(t *testing.T) {
		backend, clock, _ := newBackend(t, factory)
		ctx := context.Background()
		low := enqueue(t, backend, work.Request{Kind: "mail.send", Priority: -1})
		high := enqueue(t, backend, work.Request{Kind: "mail.send", Priority: 10})
		future := enqueue(t, backend, work.Request{
			Kind: "mail.send", Priority: 100, RunAt: clock.Now().Add(time.Hour),
		})
		claim := claim(t, backend, "delivery", "worker-a")
		if claim.Job.ID != high.ID {
			t.Fatalf("priority ordering drifted: got %s want %s", claim.Job.ID, high.ID)
		}
		other, err := backend.Claim(ctx, work.ClaimRequest{
			Queue: "delivery", WorkerID: "worker-b", LeaseDuration: 30 * time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !other.Found || other.Lease.Job.ID != low.ID {
			t.Fatalf("second worker must claim another job: %+v", other)
		}
		third, err := backend.Claim(ctx, work.ClaimRequest{
			Queue: "delivery", WorkerID: "worker-c", LeaseDuration: 30 * time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
		if third.Found {
			t.Fatalf("future job %s was claimed early: %+v", future.ID, third)
		}
	})

	t.Run("progress completion and attempt history", func(t *testing.T) {
		backend, _, _ := newBackend(t, factory)
		ctx := context.Background()
		job := enqueue(t, backend, work.Request{Kind: "asset.rebuild"})
		lease := claim(t, backend, "maintenance", "worker-a")
		if lease.Job.ID != job.ID {
			t.Fatalf("claimed wrong job: %+v", lease.Job)
		}
		if err := backend.ReportProgress(ctx, lease, json.RawMessage(`{"done":3}`)); err != nil {
			t.Fatal(err)
		}
		completed, err := backend.Complete(ctx, work.CompleteCommand{
			Lease:  lease,
			Result: work.Result{Summary: "rebuilt", Data: json.RawMessage(`{"count":3}`)},
		})
		if err != nil {
			t.Fatal(err)
		}
		if completed.Status != work.StatusSucceeded || completed.ResultSummary != "rebuilt" ||
			string(completed.Progress) != `{"done":3}` {
			t.Fatalf("unexpected completed job: %+v", completed)
		}
		attempts, err := backend.Attempts(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(attempts) != 1 || attempts[0].Outcome != work.AttemptSucceeded ||
			attempts[0].FinishedAt == nil {
			t.Fatalf("unexpected attempt history: %+v", attempts)
		}
		if _, err := backend.Complete(ctx, work.CompleteCommand{Lease: lease}); !work.IsKind(err, work.ErrorLeaseLost) {
			t.Fatalf("stale completion must lose the lease, got %v", err)
		}
	})

	t.Run("retry and expired lease recovery", func(t *testing.T) {
		backend, clock, catalog := newBackend(t, factory)
		ctx := context.Background()
		job := enqueue(t, backend, work.Request{Kind: "mail.send", MaxAttempts: 2})
		first := claim(t, backend, "delivery", "worker-a")
		retryAt := clock.Now().Add(time.Minute)
		retrying, err := backend.Retry(ctx, work.RetryCommand{
			Lease: first, Error: "provider unavailable", RunAt: retryAt,
		})
		if err != nil {
			t.Fatal(err)
		}
		if retrying.Status != work.StatusRetrying || !retrying.RunAt.Equal(retryAt) {
			t.Fatalf("unexpected retry state: %+v", retrying)
		}
		clock.Set(retryAt)
		second := claim(t, backend, "delivery", "worker-b")
		clock.Add(31 * time.Second)
		none, err := backend.Claim(ctx, work.ClaimRequest{
			Queue: "delivery", WorkerID: "worker-c", LeaseDuration: 30 * time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
		if none.Found {
			t.Fatalf("exhausted expired job must not be reclaimed immediately: %+v", none)
		}
		failed, err := backend.Get(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if failed.Status != work.StatusFailed || failed.LastError != "lease expired" {
			t.Fatalf("expired final lease must fail: %+v", failed)
		}
		if _, err := backend.Heartbeat(ctx, second, catalog.Limits().MinLease); !work.IsKind(err, work.ErrorLeaseLost) {
			t.Fatalf("expired owner must lose the lease, got %v", err)
		}
	})

	t.Run("pause cancel replay and prune", func(t *testing.T) {
		backend, clock, _ := newBackend(t, factory)
		ctx := context.Background()
		job := enqueue(t, backend, work.Request{Kind: "asset.rebuild"})
		paused, err := backend.Pause(ctx, job.ID)
		if err != nil || paused.Status != work.StatusPaused {
			t.Fatalf("pause failed: %+v err=%v", paused, err)
		}
		resumed, err := backend.Resume(ctx, job.ID)
		if err != nil || resumed.Status != work.StatusQueued {
			t.Fatalf("resume failed: %+v err=%v", resumed, err)
		}
		cancelled, err := backend.Cancel(ctx, job.ID)
		if err != nil || cancelled.Status != work.StatusCancelled {
			t.Fatalf("cancel failed: %+v err=%v", cancelled, err)
		}
		replay, err := backend.Replay(ctx, job.ID, work.ReplayRequest{
			IdempotencyKey: "replay:000000000001",
		})
		if err != nil {
			t.Fatal(err)
		}
		if replay.Job.ID == job.ID || replay.Job.ReplayOf != job.ID || replay.Job.Status != work.StatusQueued {
			t.Fatalf("unexpected replay job: %+v", replay)
		}
		other := enqueue(t, backend, work.Request{Kind: "asset.rebuild"})
		if _, err := backend.Cancel(ctx, other.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := backend.Replay(ctx, other.ID, work.ReplayRequest{
			IdempotencyKey: "replay:000000000001",
		}); !work.IsKind(err, work.ErrorConflict) {
			t.Fatalf("one replay key must not change source jobs, got %v", err)
		}
		clock.Add(time.Hour)
		pruned, err := backend.Prune(ctx, clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		if pruned.JobsRemoved != 2 {
			t.Fatalf("expected terminal sources to be pruned: %+v", pruned)
		}
		if _, err := backend.Get(ctx, replay.Job.ID); err != nil {
			t.Fatalf("live replay was pruned: %v", err)
		}
	})

	t.Run("recurring schedule materialization is exact", func(t *testing.T) {
		backend, clock, _ := newBackend(t, factory)
		ctx := context.Background()
		clock.Set(time.Date(2026, 7, 23, 12, 1, 1, 0, time.UTC))
		count, err := backend.MaterializeSchedules(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("expected one occurrence, got %d", count)
		}
		again, err := backend.MaterializeSchedules(ctx)
		if err != nil || again != 0 {
			t.Fatalf("same occurrence must be exact: count=%d err=%v", again, err)
		}
		jobs, err := backend.List(ctx, work.ListQuery{Kinds: []work.Kind{"maintenance.sweep"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) != 1 || jobs[0].IdempotencyKey != "schedule:hourly-sweep:1784808060" {
			t.Fatalf("unexpected scheduled job: %+v", jobs)
		}
	})

	t.Run("concurrent idempotency is exact", func(t *testing.T) {
		backend, _, _ := newBackend(t, factory)
		ctx := context.Background()
		const count = 24
		var wait sync.WaitGroup
		wait.Add(count)
		ids := make(chan work.JobID, count)
		errs := make(chan error, count)
		for range count {
			go func() {
				defer wait.Done()
				result, err := backend.Enqueue(ctx, work.Request{
					Kind: "mail.send", Payload: json.RawMessage(`{"message":"same"}`),
					IdempotencyKey: "concurrent:00000001",
				})
				if err != nil {
					errs <- err
					return
				}
				ids <- result.Job.ID
			}()
		}
		wait.Wait()
		close(ids)
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		var expected work.JobID
		for id := range ids {
			if expected == "" {
				expected = id
			}
			if id != expected {
				t.Fatalf("idempotent concurrency created multiple jobs: %s and %s", expected, id)
			}
		}
	})
}

func newBackend(t *testing.T, factory Factory) (work.Backend, *Clock, *work.Catalog) {
	t.Helper()
	clock := NewClock(time.Date(2026, 7, 23, 12, 0, 30, 0, time.UTC))
	catalog, err := work.Compile(work.Definition{
		Version: work.DefinitionVersion,
		Queues: []work.QueueDefinition{
			{Key: "delivery", Concurrency: 4},
			{Key: "maintenance", Concurrency: 2},
		},
		Kinds: []work.KindDefinition{
			{Key: "mail.send", Queue: "delivery", DefaultAttempts: 3, MaxAttempts: 10},
			{Key: "asset.rebuild", Queue: "maintenance", DefaultAttempts: 3, MaxAttempts: 10},
			{Key: "maintenance.sweep", Queue: "maintenance", DefaultAttempts: 1, MaxAttempts: 3},
		},
		Schedules: []work.ScheduleDefinition{
			{Key: "hourly-sweep", Cron: "1 * * * *", Kind: "maintenance.sweep"},
		},
		Retry: work.RetryPolicy{BaseDelay: time.Second, MaxDelay: time.Minute, Jitter: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	return factory(t, catalog, clock), clock, catalog
}

func enqueue(t *testing.T, backend work.Backend, request work.Request) work.Job {
	t.Helper()
	result, err := backend.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return result.Job
}

func claim(t *testing.T, backend work.Backend, queue work.Queue, worker string) work.Lease {
	t.Helper()
	result, err := backend.Claim(context.Background(), work.ClaimRequest{
		Queue: queue, WorkerID: worker, LeaseDuration: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found {
		t.Fatal("expected a due job")
	}
	return result.Lease
}
