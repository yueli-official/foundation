package ghttpx

import (
	"fmt"
	"os"
	"strconv"
	"time"

	foundationratelimit "github.com/yueli-official/foundation/go/goframe/ratelimit"
)

const defaultRateLimitPerMinute = 600

type RateLimiter struct {
	inner *foundationratelimit.Limiter
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{inner: foundationratelimit.MustNew(foundationratelimit.Policy{Limit: limit, Window: window})}
}

func (value *RateLimiter) Evaluate(key string) foundationratelimit.Decision {
	return value.inner.Evaluate(key)
}

// RateLimiterFromEnvironment is an explicit Platform process-entry-point
// adapter. Foundation never reads PLATFORM_* variables or installs a global
// policy at import time.
func RateLimiterFromEnvironment() (*RateLimiter, error) {
	limit := defaultRateLimitPerMinute
	if raw := os.Getenv("PLATFORM_RATE_LIMIT_PER_MINUTE"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			return nil, fmt.Errorf("PLATFORM_RATE_LIMIT_PER_MINUTE must be a non-negative integer")
		}
		limit = parsed
	}
	inner, err := foundationratelimit.New(foundationratelimit.Policy{Limit: limit, Window: time.Minute})
	if err != nil {
		return nil, err
	}
	return &RateLimiter{inner: inner}, nil
}

func MustRateLimiterFromEnvironment() *RateLimiter {
	value, err := RateLimiterFromEnvironment()
	if err != nil {
		panic(err)
	}
	return value
}
