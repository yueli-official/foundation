package ghttpx

import (
	"os"
	"strconv"
	"sync"
	"time"
)

const defaultRateLimitPerMinute = 600
const defaultRateLimitMaxClients = 100_000

type rateBucket struct {
	started time.Time
	count   int
}

// RateLimiter is an in-process fixed-window limiter. Each API process applies
// it after Caddy has forwarded the original client IP. Multi-node deployments
// must replace it with a shared edge limiter before scaling beyond Compose.
type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	maxKeys int
	buckets map[string]rateBucket
	cleaned time.Time
	now     func() time.Time
}

// NewRateLimiter creates a limiter. A non-positive limit disables limiting.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, maxKeys: defaultRateLimitMaxClients, buckets: map[string]rateBucket{}, now: time.Now}
}

// Allow records one request and reports whether it is allowed, its remaining
// quota, and when the current window resets.
func (l *RateLimiter) Allow(key string) (bool, int, time.Time) {
	now := l.now()
	if l.limit <= 0 {
		return true, -1, now.Add(l.window)
	}
	if key == "" {
		key = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cleaned.IsZero() || now.Sub(l.cleaned) >= l.window {
		for existingKey, existing := range l.buckets {
			if now.Sub(existing.started) >= l.window {
				delete(l.buckets, existingKey)
			}
		}
		l.cleaned = now
	}
	bucket, exists := l.buckets[key]
	if !exists && len(l.buckets) >= l.maxKeys {
		return false, 0, now.Add(l.window)
	}
	if bucket.started.IsZero() || now.Sub(bucket.started) >= l.window {
		bucket = rateBucket{started: now}
	}
	bucket.count++
	l.buckets[key] = bucket
	remaining := max(0, l.limit-bucket.count)
	return bucket.count <= l.limit, remaining, bucket.started.Add(l.window)
}

func rateLimiterFromEnvironment() *RateLimiter {
	limit := defaultRateLimitPerMinute
	if raw := os.Getenv("PLATFORM_RATE_LIMIT_PER_MINUTE"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			limit = parsed
		}
	}
	return NewRateLimiter(limit, time.Minute)
}
