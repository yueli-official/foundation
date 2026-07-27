package api_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	goframeapi "github.com/yueli-official/foundation/go/goframe/api"
	"github.com/yueli-official/foundation/go/goframe/ratelimit"
	"github.com/yueli-official/foundation/go/problem"
)

var (
	rateLimited = problem.MustDescriptor(
		problem.MustKind("common.rate_limited", http.StatusTooManyRequests),
		"https://errors.example.test/problems/common.rate_limited",
	)
	validation = problem.MustDescriptor(
		problem.MustKind("common.validation_failed", http.StatusBadRequest),
		"https://errors.example.test/problems/common.validation_failed",
	)
	internal = problem.MustDescriptor(
		problem.MustKind("common.internal", http.StatusInternalServerError),
		"https://errors.example.test/problems/common.internal",
	)
	conflict = problem.MustDescriptor(
		problem.MustKind("example.conflict", http.StatusConflict),
		"https://errors.example.test/problems/example.conflict",
	)
)

func TestMiddlewareWritesMappedErrorsWithoutLeakingUnknownFailures(t *testing.T) {
	middleware := newMiddleware(t, nil)
	server := g.Server(t.Name())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Use(middleware.Handle)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.GET("/mapped", func(request *ghttp.Request) {
			failure, err := problem.NewError(conflict, problem.Parameters{"key": "value"})
			if err != nil {
				t.Fatal(err)
			}
			request.SetError(failure)
		})
		group.GET("/unknown", func(request *ghttp.Request) {
			request.SetError(errors.New("secret database diagnostic"))
		})
	})
	server.Start()
	defer server.Shutdown()

	client := g.Client()
	client.SetPrefix(fmt.Sprintf("http://127.0.0.1:%d", server.GetListenedPort()))
	mapped, err := client.Get(context.Background(), "/mapped")
	if err != nil {
		t.Fatal(err)
	}
	mappedBody := mapped.ReadAllString()
	mapped.Close()
	if mapped.StatusCode != http.StatusConflict || !strings.Contains(mappedBody, `"code":"example.conflict"`) {
		t.Fatalf("mapped response = %d %s", mapped.StatusCode, mappedBody)
	}
	unknown, err := client.Get(context.Background(), "/unknown")
	if err != nil {
		t.Fatal(err)
	}
	body := unknown.ReadAllString()
	unknown.Close()
	if unknown.StatusCode != http.StatusInternalServerError || strings.Contains(body, "secret database diagnostic") {
		t.Fatalf("unknown response = %d %s", unknown.StatusCode, body)
	}
}

func TestMiddlewareAppliesCallerOwnedRateLimit(t *testing.T) {
	limiter := ratelimit.MustNew(ratelimit.Policy{Limit: 1, Window: time.Minute})
	middleware := newMiddleware(t, limiter)
	server := g.Server(t.Name())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Use(middleware.Handle)
	server.BindHandler("GET:/", func(request *ghttp.Request) { request.Response.Write("ok") })
	server.Start()
	defer server.Shutdown()

	client := g.Client()
	client.SetPrefix(fmt.Sprintf("http://127.0.0.1:%d", server.GetListenedPort()))
	first, err := client.Get(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	second, err := client.Get(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.StatusCode != http.StatusTooManyRequests || second.Header.Get("RateLimit-Limit") != "1" {
		t.Fatalf("rate-limited response = %d %+v", second.StatusCode, second.Header)
	}
}

func newMiddleware(t *testing.T, limiter *ratelimit.Limiter) *goframeapi.Middleware {
	t.Helper()
	options := goframeapi.Options{
		Limiter: limiter, RateLimited: rateLimited, Validation: validation, Internal: internal,
	}
	if limiter != nil {
		options.ClientKey = func(*ghttp.Request) string { return "client" }
	}
	middleware, err := goframeapi.New(options)
	if err != nil {
		t.Fatal(err)
	}
	return middleware
}
