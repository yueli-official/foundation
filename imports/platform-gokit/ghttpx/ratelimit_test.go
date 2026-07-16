package ghttpx

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func TestRateLimiterEnforcesAndResetsWindow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := NewRateLimiter(2, time.Minute)
	limiter.now = func() time.Time { return now }
	for index, wantAllowed := range []bool{true, true, false} {
		allowed, remaining, reset := limiter.Allow("203.0.113.10")
		if allowed != wantAllowed {
			t.Fatalf("request %d allowed = %v, want %v", index+1, allowed, wantAllowed)
		}
		if remaining != max(0, 1-index) || !reset.Equal(now.Add(time.Minute)) {
			t.Fatalf("request %d remaining/reset = %d/%v", index+1, remaining, reset)
		}
	}
	now = now.Add(time.Minute)
	allowed, remaining, _ := limiter.Allow("203.0.113.10")
	if !allowed || remaining != 1 {
		t.Fatalf("new window allowed/remaining = %v/%d", allowed, remaining)
	}
}

func TestMiddlewareReturnsRateLimitEnvelopeAndHeaders(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute)
	server := g.Server(t.Name())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(request *ghttp.Request) { middleware(limiter, request) })
		group.GET("/limited", func(request *ghttp.Request) { request.Response.Write("ok") })
	})
	server.Start()
	defer server.Shutdown()
	client := g.Client()
	client.SetPrefix(fmt.Sprintf("http://127.0.0.1:%d", server.GetListenedPort()))
	first, err := client.Get(context.Background(), "/limited")
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	second, err := client.Get(context.Background(), "/limited")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	resetAfter, parseErr := strconv.Atoi(second.Header.Get("RateLimit-Reset"))
	if second.StatusCode != 429 || second.Header.Get("Retry-After") == "" || second.Header.Get("RateLimit-Limit") != "1" || parseErr != nil || resetAfter < 1 || resetAfter > 60 {
		t.Fatalf("status/headers = %d %+v", second.StatusCode, second.Header)
	}
	if body := second.ReadAllString(); !strings.Contains(body, `"code":"common.rate_limited"`) || !strings.Contains(body, `"traceId"`) {
		t.Fatalf("unexpected body %s", body)
	}
}

func TestRawMiddlewareReturnsOAuthRateLimitShape(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute)
	server := g.Server(t.Name())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(request *ghttp.Request) { rawRateLimitMiddleware(limiter, request) })
		group.POST("/oauth2/token", func(request *ghttp.Request) { request.Response.WriteJson(map[string]string{"access_token": "test"}) })
	})
	server.Start()
	defer server.Shutdown()
	client := g.Client()
	client.SetPrefix(fmt.Sprintf("http://127.0.0.1:%d", server.GetListenedPort()))
	first, err := client.Post(context.Background(), "/oauth2/token")
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	second, err := client.Post(context.Background(), "/oauth2/token")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	body := second.ReadAllString()
	if second.StatusCode != 429 || !strings.Contains(body, `"error":"temporarily_unavailable"`) || strings.Contains(body, `"code":"common.rate_limited"`) {
		t.Fatalf("status/body = %d %s", second.StatusCode, body)
	}
}

func TestRateLimiterSeparatesClientsAndIsConcurrent(t *testing.T) {
	limiter := NewRateLimiter(10, time.Minute)
	var allowed atomic.Int64
	var wait sync.WaitGroup
	for index := range 40 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if ok, _, _ := limiter.Allow("client-" + string(rune('a'+index%2))); ok {
				allowed.Add(1)
			}
		}()
	}
	wait.Wait()
	if got := allowed.Load(); got != 20 {
		t.Fatalf("allowed = %d, want 20", got)
	}
}

func TestRateLimiterCanBeExplicitlyDisabled(t *testing.T) {
	limiter := NewRateLimiter(0, time.Minute)
	for range 1000 {
		if allowed, remaining, _ := limiter.Allow("client"); !allowed || remaining != -1 {
			t.Fatalf("disabled limiter allowed/remaining = %v/%d", allowed, remaining)
		}
	}
}

func TestRateLimiterBoundsDistinctClientState(t *testing.T) {
	limiter := NewRateLimiter(10, time.Minute)
	limiter.maxKeys = 2
	for _, client := range []string{"one", "two"} {
		if allowed, _, _ := limiter.Allow(client); !allowed {
			t.Fatalf("client %s unexpectedly denied", client)
		}
	}
	if allowed, _, _ := limiter.Allow("three"); allowed {
		t.Fatal("third distinct client should fail closed at state cap")
	}
}

func TestMediaMiddlewareDoesNotConsumeDefaultAPIRateBucket(t *testing.T) {
	server := g.Server(t.Name())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(MediaMiddleware)
		group.GET("/media", func(request *ghttp.Request) { request.Response.Write("image") })
	})
	server.Start()
	defer server.Shutdown()
	client := g.Client()
	client.SetPrefix(fmt.Sprintf("http://127.0.0.1:%d", server.GetListenedPort()))
	for request := 1; request <= defaultRateLimitPerMinute+20; request++ {
		response, err := client.Get(context.Background(), "/media")
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK || response.Header.Get("RateLimit-Limit") != "" {
			response.Close()
			t.Fatalf("media request %d status/rate-limit = %d/%q", request, response.StatusCode, response.Header.Get("RateLimit-Limit"))
		}
		response.Close()
	}
}
