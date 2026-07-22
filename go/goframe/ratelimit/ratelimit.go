// Package ratelimit provides an explicitly configured, bounded fixed-window
// policy for GoFrame HTTP adapters. Callers own the client-key extraction and
// therefore must make their trusted-proxy topology explicit.
package ratelimit

import (
	"errors"
	"math"
	"strconv"
	"sync"
	"time"
)

const defaultMaxKeys = 100_000

type Policy struct {
	Limit   int
	Window  time.Duration
	MaxKeys int
}

type bucket struct {
	started time.Time
	count   int
}

type Limiter struct {
	mu      sync.Mutex
	policy  Policy
	buckets map[string]bucket
	cleaned time.Time
	now     func() time.Time
}

type Decision struct {
	Allowed    bool
	Enabled    bool
	Limit      int
	Remaining  int
	ResetAfter time.Duration
}

func New(policy Policy) (*Limiter, error) {
	if policy.Limit < 0 {
		return nil, errors.New("rate limit must not be negative")
	}
	if policy.Limit > 0 && policy.Window <= 0 {
		return nil, errors.New("rate-limit window must be positive")
	}
	if policy.MaxKeys < 0 {
		return nil, errors.New("rate-limit max keys must not be negative")
	}
	if policy.MaxKeys == 0 {
		policy.MaxKeys = defaultMaxKeys
	}
	return &Limiter{policy: policy, buckets: map[string]bucket{}, now: time.Now}, nil
}

func MustNew(policy Policy) *Limiter {
	value, err := New(policy)
	if err != nil {
		panic(err)
	}
	return value
}

// Evaluate consumes one unit for key. An empty key is intentionally grouped
// into a single unknown bucket rather than bypassing the policy.
func (value *Limiter) Evaluate(key string) Decision {
	now := value.now()
	if value.policy.Limit == 0 {
		return Decision{Allowed: true}
	}
	if key == "" {
		key = "unknown"
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.cleaned.IsZero() || now.Sub(value.cleaned) >= value.policy.Window {
		for existingKey, existing := range value.buckets {
			if now.Sub(existing.started) >= value.policy.Window {
				delete(value.buckets, existingKey)
			}
		}
		value.cleaned = now
	}
	current, exists := value.buckets[key]
	if !exists && len(value.buckets) >= value.policy.MaxKeys {
		return Decision{Allowed: false, Enabled: true, Limit: value.policy.Limit, Remaining: 0, ResetAfter: value.policy.Window}
	}
	if current.started.IsZero() || now.Sub(current.started) >= value.policy.Window {
		current = bucket{started: now}
	}
	current.count++
	value.buckets[key] = current
	return Decision{
		Allowed: current.count <= value.policy.Limit, Enabled: true, Limit: value.policy.Limit,
		Remaining: max(0, value.policy.Limit-current.count), ResetAfter: max(time.Nanosecond, current.started.Add(value.policy.Window).Sub(now)),
	}
}

// Headers returns RFC-compatible quota headers without mutating a response.
func (value Decision) Headers() map[string]string {
	if !value.Enabled {
		return nil
	}
	seconds := max(1, int(math.Ceil(value.ResetAfter.Seconds())))
	headers := map[string]string{
		"RateLimit-Limit": strconv.Itoa(value.Limit), "RateLimit-Remaining": strconv.Itoa(value.Remaining),
		"RateLimit-Reset": strconv.Itoa(seconds),
	}
	if !value.Allowed {
		headers["Retry-After"] = strconv.Itoa(seconds)
	}
	return headers
}
