package ghttpx

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func TestMiddlewareReturnsRateLimitEnvelopeAndHeaders(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute)
	server := g.Server(t.Name())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(request *ghttp.Request) { middleware(limiter, ForwardedClientIPKey, request) })
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
		group.Middleware(func(request *ghttp.Request) { rawRateLimitMiddleware(limiter, ForwardedClientIPKey, request) })
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

func TestEnvironmentPolicyIsExplicitAndStrict(t *testing.T) {
	t.Setenv("PLATFORM_RATE_LIMIT_PER_MINUTE", "invalid")
	if _, err := RateLimiterFromEnvironment(); err == nil {
		t.Fatal("invalid environment policy accepted")
	}
	t.Setenv("PLATFORM_RATE_LIMIT_PER_MINUTE", "0")
	limiter, err := RateLimiterFromEnvironment()
	if err != nil || !limiter.Evaluate("client").Allowed {
		t.Fatalf("disabled policy limiter=%v err=%v", limiter, err)
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
