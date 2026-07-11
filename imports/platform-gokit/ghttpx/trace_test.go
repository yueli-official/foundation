package ghttpx_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"platform/gokit/ghttpx"
	"platform/gokit/observability"
)

func TestTraceRouteMiddlewareExportsMatchedRouteAndSanitizesUnmatchedPath(t *testing.T) {
	memory := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(observability.NewSanitizingExporter(memory)))
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
	s.Use(ghttpx.TraceRouteMiddleware)
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
