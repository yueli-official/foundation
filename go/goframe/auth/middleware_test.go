package auth_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	coreauth "github.com/yueli-official/foundation/go/auth"
	goframeauth "github.com/yueli-official/foundation/go/goframe/auth"
	goframehttp "github.com/yueli-official/foundation/go/goframe/http"
	"github.com/yueli-official/foundation/go/problem"
)

type verifierFunc func(context.Context, string) (*coreauth.Principal, error)

func (function verifierFunc) Verify(ctx context.Context, raw string) (*coreauth.Principal, error) {
	return function(ctx, raw)
}

func TestMiddlewareValidatesConfiguration(t *testing.T) {
	verifier := verifierFunc(func(context.Context, string) (*coreauth.Principal, error) {
		return &coreauth.Principal{Subject: "user"}, nil
	})
	writer := goframehttp.MustWriter(goframehttp.WriterOptions{})
	kind := problem.MustKind("auth.unauthorized", http.StatusUnauthorized)
	valid := goframeauth.Options{
		Verifier: verifier, Writer: &writer, UnauthorizedKind: kind,
		UnauthorizedType: "https://example.test/problems/auth.unauthorized",
		TraceID:          func(*ghttp.Request) string { return "trace-1" },
	}
	tests := []struct {
		name   string
		mutate func(*goframeauth.Options)
	}{
		{name: "missing verifier", mutate: func(options *goframeauth.Options) { options.Verifier = nil }},
		{name: "missing writer", mutate: func(options *goframeauth.Options) { options.Writer = nil }},
		{name: "wrong status", mutate: func(options *goframeauth.Options) {
			options.UnauthorizedKind = problem.MustKind("auth.unauthorized", http.StatusForbidden)
		}},
		{name: "invalid type", mutate: func(options *goframeauth.Options) { options.UnauthorizedType = "not-a-uri" }},
		{name: "missing trace", mutate: func(options *goframeauth.Options) { options.TraceID = nil }},
		{name: "unsafe realm", mutate: func(options *goframeauth.Options) { options.Realm = "bad\nrealm" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := valid
			test.mutate(&options)
			if _, err := goframeauth.NewMiddleware(options); err == nil {
				t.Fatal("NewMiddleware() accepted invalid configuration")
			}
		})
	}
}

func TestMiddlewareRequiredAndOptionalBehaviorOnRealServer(t *testing.T) {
	server, baseURL := startServer(t)
	defer server.Shutdown()

	tests := []struct {
		name          string
		path          string
		authorization []string
		wantStatus    int
		wantBody      string
		wantChallenge string
	}{
		{name: "required missing", path: "/required", wantStatus: 401, wantBody: "auth.unauthorized", wantChallenge: `Bearer realm="foundation"`},
		{name: "required malformed", path: "/required", authorization: []string{"Basic value"}, wantStatus: 401, wantBody: "auth.unauthorized", wantChallenge: `error="invalid_token"`},
		{name: "required multiple", path: "/required", authorization: []string{"Bearer valid", "Bearer valid"}, wantStatus: 401, wantBody: "auth.unauthorized", wantChallenge: `error="invalid_token"`},
		{name: "required invalid", path: "/required", authorization: []string{"Bearer invalid"}, wantStatus: 401, wantBody: "auth.unauthorized", wantChallenge: `error="invalid_token"`},
		{name: "required valid", path: "/required", authorization: []string{"Bearer valid"}, wantStatus: 200, wantBody: "user-1"},
		{name: "optional missing", path: "/optional", wantStatus: 200, wantBody: "anonymous"},
		{name: "optional invalid", path: "/optional", authorization: []string{"Bearer invalid"}, wantStatus: 401, wantBody: "auth.unauthorized", wantChallenge: `error="invalid_token"`},
		{name: "optional valid", path: "/optional", authorization: []string{"bearer valid"}, wantStatus: 200, wantBody: "user-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+test.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range test.authorization {
				request.Header.Add("Authorization", value)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != test.wantStatus || !contains(string(body), test.wantBody) {
				t.Fatalf("status = %d, body = %s", response.StatusCode, body)
			}
			if test.wantChallenge != "" && !contains(response.Header.Get("WWW-Authenticate"), test.wantChallenge) {
				t.Fatalf("WWW-Authenticate = %q", response.Header.Get("WWW-Authenticate"))
			}
			if test.wantStatus == 401 {
				if response.Header.Get("Content-Type") != "application/problem+json" || response.Header.Get("X-Trace-Id") != "trace-auth-1" {
					t.Fatalf("problem headers = %#v", response.Header)
				}
				if _, err := problem.Decode(body); err != nil {
					t.Fatalf("invalid Problem body: %v", err)
				}
			}
		})
	}
}

func startServer(t *testing.T) (*ghttp.Server, string) {
	t.Helper()
	verifier := verifierFunc(func(_ context.Context, raw string) (*coreauth.Principal, error) {
		if raw != "valid" {
			return nil, errors.New("invalid token details must not leak")
		}
		return &coreauth.Principal{Subject: "user-1"}, nil
	})
	writer := goframehttp.MustWriter(goframehttp.WriterOptions{})
	middleware, err := goframeauth.NewMiddleware(goframeauth.Options{
		Verifier: verifier,
		Writer:   &writer,
		UnauthorizedKind: problem.MustKind(
			"auth.unauthorized",
			http.StatusUnauthorized,
		),
		UnauthorizedType: "https://example.test/problems/auth.unauthorized",
		TraceID:          func(*ghttp.Request) string { return "trace-auth-1" },
		Realm:            "foundation",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := g.Server(t.Name())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/required", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.Required)
		group.GET("/", principalHandler)
	})
	server.Group("/optional", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.Optional)
		group.GET("/", principalHandler)
	})
	server.Start()
	return server, fmt.Sprintf("http://127.0.0.1:%d", server.GetListenedPort())
}

func principalHandler(request *ghttp.Request) {
	principal, ok := coreauth.FromContext(request.Context())
	if !ok {
		request.Response.Write("anonymous")
		return
	}
	request.Response.Write(principal.ActorKey())
}

func contains(value, fragment string) bool {
	return strings.Contains(value, fragment)
}
