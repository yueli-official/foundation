package discovery

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheUsesExplicitTTLAndSingleFlightRefresh(t *testing.T) {
	module := testModule(t)
	now := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	var builds atomic.Int64
	cache, err := NewCache(module, CacheOptions{
		TTL: 5 * time.Minute,
		Clock: func() time.Time {
			return now
		},
		Build: func(context.Context) (PublicationPlan, Sources, error) {
			builds.Add(1)
			return PublicationPlan{Robots: &RobotsPlan{}}, nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	const workers = 12
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			snapshot, err := cache.Snapshot(context.Background())
			if err != nil || len(snapshot.Artifacts["robots.txt"]) == 0 {
				t.Errorf("snapshot failed: %v", err)
			}
		}()
	}
	wait.Wait()
	if builds.Load() != 1 {
		t.Fatalf("expected one refresh, got %d", builds.Load())
	}
	now = now.Add(5 * time.Minute)
	if _, err := cache.Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if builds.Load() != 2 {
		t.Fatalf("expected refresh after expiry, got %d builds", builds.Load())
	}
}
