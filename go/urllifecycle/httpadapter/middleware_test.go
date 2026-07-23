package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/urllifecycle"
)

type resolverFunc func(context.Context, urllifecycle.Lookup) (urllifecycle.Resolution, error)

func (function resolverFunc) Resolve(ctx context.Context, lookup urllifecycle.Lookup) (urllifecycle.Resolution, error) {
	return function(ctx, lookup)
}

func TestMiddlewareUsesTrustedOriginAndExplicitCache(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	expires := now.Add(10 * time.Second)
	middleware, err := Middleware(resolverFunc(func(_ context.Context, lookup urllifecycle.Lookup) (urllifecycle.Resolution, error) {
		if lookup.EscapedPath != "/old" || lookup.RawQuery != "utm=x" {
			t.Fatalf("unexpected lookup: %#v", lookup)
		}
		return urllifecycle.Resolution{
			Kind: urllifecycle.ResolutionRedirect, Location: "/new?utm=x",
			StatusCode: http.StatusTemporaryRedirect, ExpiresAt: &expires,
		}, nil
	}), Options{
		TrustedOrigin: "https://canonical.example.test",
		Cache:         CachePolicy{Temporary: time.Minute},
		Clock:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "https://evil.example/old?utm=x", nil)
	request.Host = "evil.example"
	response := httptest.NewRecorder()
	middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("redirect called next handler")
	})).ServeHTTP(response, request)
	if response.Code != http.StatusTemporaryRedirect {
		t.Fatalf("got %d", response.Code)
	}
	if got := response.Header().Get("Location"); got != "https://canonical.example.test/new?utm=x" {
		t.Fatalf("request Host influenced Location: %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "public, max-age=10" {
		t.Fatalf("temporary expiry did not bound cache: %q", got)
	}
}

func TestMiddlewareGoneUnknownAndFailure(t *testing.T) {
	tests := []struct {
		name       string
		resolution urllifecycle.Resolution
		err        error
		wantStatus int
		wantNext   bool
	}{
		{name: "gone", resolution: urllifecycle.Resolution{Kind: urllifecycle.ResolutionGone}, wantStatus: 410},
		{name: "unknown", resolution: urllifecycle.Resolution{Kind: urllifecycle.ResolutionUnknown}, wantStatus: 204, wantNext: true},
		{name: "failure", err: errors.New("database down"), wantStatus: 503},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			middleware, err := Middleware(resolverFunc(func(context.Context, urllifecycle.Lookup) (urllifecycle.Resolution, error) {
				return test.resolution, test.err
			}), Options{TrustedOrigin: "https://example.test"})
			if err != nil {
				t.Fatal(err)
			}
			called := false
			next := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				called = true
				writer.WriteHeader(204)
			})
			response := httptest.NewRecorder()
			middleware(next).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://request.invalid/path", nil))
			if response.Code != test.wantStatus || called != test.wantNext {
				t.Fatalf("status=%d called=%t", response.Code, called)
			}
			if test.name != "unknown" && response.Header().Get("Cache-Control") == "" {
				t.Fatal("terminal response omitted Cache-Control")
			}
		})
	}
}

func TestMiddlewareRejectsUnsafeResolverLocation(t *testing.T) {
	middleware, err := Middleware(resolverFunc(func(context.Context, urllifecycle.Lookup) (urllifecycle.Resolution, error) {
		return urllifecycle.Resolution{
			Kind: urllifecycle.ResolutionRedirect, Location: "//evil.example/path",
			StatusCode: http.StatusPermanentRedirect,
		}, nil
	}), Options{TrustedOrigin: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	middleware(http.NotFoundHandler()).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "https://example.test/old", nil),
	)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unsafe Location was emitted: %d %q", response.Code, response.Header().Get("Location"))
	}
}

func TestMiddlewareEmitsEveryLifecycleRedirectStatus(t *testing.T) {
	for _, status := range []int{
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			middleware, err := Middleware(resolverFunc(func(context.Context, urllifecycle.Lookup) (urllifecycle.Resolution, error) {
				return urllifecycle.Resolution{
					Kind:     urllifecycle.ResolutionRedirect,
					Location: "/target", StatusCode: status,
				}, nil
			}), Options{TrustedOrigin: "https://example.test"})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			middleware(http.NotFoundHandler()).ServeHTTP(
				response,
				httptest.NewRequest(http.MethodPost, "https://request.invalid/source", nil),
			)
			if response.Code != status ||
				response.Header().Get("Location") != "https://example.test/target" ||
				response.Header().Get("Cache-Control") == "" {
				t.Fatalf("status contract drifted: %d %#v", response.Code, response.Header())
			}
		})
	}
}
