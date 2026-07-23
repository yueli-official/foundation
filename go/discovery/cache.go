package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type PublicationBuilder func(context.Context) (PublicationPlan, Sources, error)

type CacheOptions struct {
	TTL   time.Duration
	Clock func() time.Time
	Build PublicationBuilder
}

// Cache keeps one complete instance-local publication snapshot. Refresh is
// single-flight and only replaces the visible snapshot after Publish commits.
// It deliberately has no distributed coordination or hidden wall clock.
type Cache struct {
	mu      sync.Mutex
	module  *Module
	ttl     time.Duration
	clock   func() time.Time
	build   PublicationBuilder
	target  *MemoryTarget
	loaded  bool
	expires time.Time
}

func NewCache(module *Module, options CacheOptions) (*Cache, error) {
	if module == nil {
		return nil, failure(ErrorConfiguration, "module_required", "module", "is required")
	}
	if options.TTL <= 0 || options.TTL > 24*time.Hour {
		return nil, failure(ErrorConfiguration, "invalid_cache_ttl", "ttl", "must be between zero and 24 hours")
	}
	if options.Clock == nil {
		return nil, failure(ErrorConfiguration, "cache_clock_required", "clock", "is required")
	}
	if options.Build == nil {
		return nil, failure(ErrorConfiguration, "publication_builder_required", "build", "is required")
	}
	return &Cache{
		module: module, ttl: options.TTL, clock: options.Clock,
		build: options.Build, target: &MemoryTarget{},
	}, nil
}

func (cache *Cache) Snapshot(ctx context.Context) (MemoryPublication, error) {
	if cache == nil {
		return MemoryPublication{}, failure(ErrorConfiguration, "cache_required", "cache", "is required")
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	now := cache.clock()
	if cache.loaded && now.Before(cache.expires) {
		return cache.target.Snapshot(), nil
	}
	plan, sources, err := cache.build(ctx)
	if err != nil {
		return MemoryPublication{}, &Error{
			Kind: ErrorSource, Code: "publication_build_failed",
			Retryable: true, Cause: err,
		}
	}
	target := &MemoryTarget{}
	if _, _, err := cache.module.Publish(ctx, plan, sources, target); err != nil {
		return MemoryPublication{}, err
	}
	snapshot := target.Snapshot()
	if len(snapshot.Artifacts) == 0 {
		return MemoryPublication{}, &Error{
			Kind: ErrorEncoding, Code: "empty_publication",
			Cause: fmt.Errorf("publication contains no artifacts"),
		}
	}
	cache.target = target
	cache.loaded = true
	cache.expires = now.Add(cache.ttl)
	return snapshot, nil
}

// Invalidate expires the local snapshot. The next Snapshot performs the
// refresh; existing bytes are not exposed as a successful refresh.
func (cache *Cache) Invalidate() {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.expires = time.Time{}
}
