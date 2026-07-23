package work_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/work"
	"github.com/yueli-official/foundation/go/work/worktest"
)

func TestRunnerRetriesThenCompletes(t *testing.T) {
	clock := worktest.NewClock(time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC))
	catalog := runnerCatalog()
	backend, err := work.NewMemory(catalog, work.MemoryOptions{Clock: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	runner, err := work.NewRunner(catalog, backend, map[work.Kind]work.Handler{
		"mail.send": work.HandlerFunc(func(
			ctx context.Context, job work.Job, progress work.Progress,
		) (work.Result, error) {
			if calls.Add(1) == 1 {
				if err := progress.Save(ctx, json.RawMessage(`{"phase":"provider"}`)); err != nil {
					return work.Result{}, err
				}
				return work.Result{}, errors.New("temporary provider failure")
			}
			return work.Result{Summary: "sent", Data: json.RawMessage(`{"providerId":"p-1"}`)}, nil
		}),
	}, work.RunnerOptions{
		WorkerID: "runner-test", LeaseDuration: 30 * time.Second,
		HeartbeatInterval: 10 * time.Second, Clock: clock.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	enqueued, err := backend.Enqueue(context.Background(), work.Request{
		Kind: "mail.send", MaxAttempts: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	worked, err := runner.RunOnce(context.Background(), "delivery", "runner-test/1")
	if err != nil || !worked {
		t.Fatalf("first run failed: worked=%v err=%v", worked, err)
	}
	retrying, err := backend.Get(context.Background(), enqueued.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retrying.Status != work.StatusRetrying || calls.Load() != 1 {
		t.Fatalf("temporary error did not retry: %+v", retrying)
	}
	clock.Set(retrying.RunAt)
	worked, err = runner.RunOnce(context.Background(), "delivery", "runner-test/1")
	if err != nil || !worked {
		t.Fatalf("second run failed: worked=%v err=%v", worked, err)
	}
	completed, err := backend.Get(context.Background(), enqueued.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != work.StatusSucceeded || completed.ResultSummary != "sent" || calls.Load() != 2 {
		t.Fatalf("retry did not complete: %+v calls=%d", completed, calls.Load())
	}
}

func TestRunnerPermanentFailureDoesNotRetry(t *testing.T) {
	clock := worktest.NewClock(time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC))
	catalog := runnerCatalog()
	backend, err := work.NewMemory(catalog, work.MemoryOptions{Clock: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := work.NewRunner(catalog, backend, map[work.Kind]work.Handler{
		"mail.send": work.HandlerFunc(func(
			context.Context, work.Job, work.Progress,
		) (work.Result, error) {
			return work.Result{}, work.Permanent(errors.New("invalid recipient"))
		}),
	}, work.RunnerOptions{
		WorkerID: "runner-test", LeaseDuration: 30 * time.Second,
		HeartbeatInterval: 10 * time.Second, Clock: clock.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	enqueued, err := backend.Enqueue(context.Background(), work.Request{Kind: "mail.send"})
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := runner.RunOnce(context.Background(), "delivery", "runner-test/1"); err != nil || !worked {
		t.Fatalf("run failed: worked=%v err=%v", worked, err)
	}
	failed, err := backend.Get(context.Background(), enqueued.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != work.StatusFailed || failed.Attempt != 1 {
		t.Fatalf("permanent failure retried: %+v", failed)
	}
}

func runnerCatalog() *work.Catalog {
	return work.MustCompile(work.Definition{
		Version: work.DefinitionVersion,
		Queues:  []work.QueueDefinition{{Key: "delivery", Concurrency: 1}},
		Kinds: []work.KindDefinition{
			{Key: "mail.send", Queue: "delivery", DefaultAttempts: 3, MaxAttempts: 10},
		},
		Retry: work.RetryPolicy{BaseDelay: time.Second, MaxDelay: time.Minute},
	})
}
