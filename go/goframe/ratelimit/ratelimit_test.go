package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestExplicitPolicyEnforcesResetsAndEmitsHeaders(t *testing.T) {
	limiter := MustNew(Policy{Limit: 2, Window: time.Minute, MaxKeys: 2})
	now := time.Unix(100, 0)
	limiter.now = func() time.Time { return now }
	first, second, third := limiter.Evaluate("client"), limiter.Evaluate("client"), limiter.Evaluate("client")
	if !first.Allowed || !second.Allowed || third.Allowed || third.Headers()["Retry-After"] != "60" {
		t.Fatalf("decisions = %+v %+v %+v", first, second, third)
	}
	now = now.Add(time.Minute)
	if reset := limiter.Evaluate("client"); !reset.Allowed || reset.Remaining != 1 {
		t.Fatalf("reset = %+v", reset)
	}
}

func TestPolicyBoundsKeysAndIsConcurrent(t *testing.T) {
	limiter := MustNew(Policy{Limit: 100, Window: time.Minute, MaxKeys: 1})
	if !limiter.Evaluate("first").Allowed || limiter.Evaluate("second").Allowed {
		t.Fatal("max-key bound not enforced")
	}
	var wait sync.WaitGroup
	for index := 0; index < 50; index++ {
		wait.Add(1)
		go func() { defer wait.Done(); limiter.Evaluate("first") }()
	}
	wait.Wait()
}

func TestPolicyRejectsImplicitOrInvalidConfiguration(t *testing.T) {
	for _, policy := range []Policy{{Limit: -1, Window: time.Minute}, {Limit: 1}, {Limit: 1, Window: time.Minute, MaxKeys: -1}} {
		if _, err := New(policy); err == nil {
			t.Fatalf("accepted %+v", policy)
		}
	}
	disabled := MustNew(Policy{})
	if decision := disabled.Evaluate(""); !decision.Allowed || decision.Enabled || decision.Headers() != nil {
		t.Fatalf("disabled = %+v", decision)
	}
}
