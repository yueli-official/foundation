package api_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	goframeapi "github.com/yueli-official/foundation/go/goframe/api"
	"github.com/yueli-official/foundation/go/problem"
	foundationtelemetry "github.com/yueli-official/foundation/go/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceRouteExportsMatchedRouteAndSanitizesUnmatchedPath(t *testing.T) {
	memory := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(foundationtelemetry.NewSanitizingExporter(memory)))
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})

	s := g.Server(t.Name())
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.Use(goframeapi.TraceRoute)
	s.BindHandler("GET:/items/{id}", func(r *ghttp.Request) { r.Response.WriteStatus(http.StatusNoContent) })
	s.Start()
	t.Cleanup(func() { _ = s.Shutdown() })

	base := fmt.Sprintf("http://127.0.0.1:%d", s.GetListenedPort())
	for _, path := range []string{"/items/42", "/missing/unmatched-path-secret-marker"} {
		response, err := http.Get(base + path) // #nosec G107 -- loopback-only integration server
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}

	names := map[string]bool{}
	for _, span := range memory.GetSpans() {
		if span.SpanKind == trace.SpanKindServer {
			names[span.Name] = true
			if strings.Contains(span.Name, "unmatched-path-secret-marker") {
				t.Fatalf("server span leaked unmatched path: %q", span.Name)
			}
		}
	}
	if !names["HTTP GET /items/{id}"] {
		t.Fatalf("matched route span missing: %v", names)
	}
	if !names["HTTP GET"] {
		t.Fatalf("low-cardinality unmatched span missing: %v", names)
	}
}

func TestTraceRouteUsesHTTPServerErrorSemantics(t *testing.T) {
	memory := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(foundationtelemetry.NewSanitizingExporter(memory)))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})

	s := g.Server(t.Name())
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.Use(goframeapi.TraceRoute)
	middleware := newMiddleware(t, nil)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.Handle)
		group.GET("/client-error", func(r *ghttp.Request) {
			failure, err := problem.NewError(validation, nil)
			if err != nil {
				t.Fatal(err)
			}
			r.SetError(failure)
		})
		group.GET("/server-error", func(r *ghttp.Request) {
			r.SetError(errors.New("internal test error"))
		})
	})
	s.Start()
	t.Cleanup(func() { _ = s.Shutdown() })

	base := fmt.Sprintf("http://127.0.0.1:%d", s.GetListenedPort())
	for path, wantStatus := range map[string]int{"/client-error": 400, "/server-error": 500} {
		response, err := http.Get(base + path) // #nosec G107 -- loopback-only integration server
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != wantStatus {
			t.Fatalf("%s status=%d, want %d", path, response.StatusCode, wantStatus)
		}
	}

	statuses := map[string]sdktrace.Status{}
	for _, span := range memory.GetSpans() {
		if span.SpanKind == trace.SpanKindServer {
			statuses[span.Name] = span.Status
		}
	}
	if got := statuses["HTTP GET /client-error"].Code; got != codes.Unset {
		t.Fatalf("client error span status=%v, want unset", got)
	}
	if got := statuses["HTTP GET /server-error"].Code; got != codes.Error {
		t.Fatalf("server error span status=%v, want error", got)
	}
}
