package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestSanitizerRemovesSecretsAndBoundsServerIdentity(t *testing.T) {
	memory := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(NewSanitizingExporter(memory)),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", "identity"), attribute.String("deployment.secret", "resource-secret"))),
	)
	_, span := provider.Tracer("test").Start(context.Background(), "/users/path-secret", trace.WithSpanKind(trace.SpanKindServer))
	span.SetAttributes(
		attribute.String("http.route", "/users/{id}"), attribute.String("http.request.method", "GET"),
		attribute.String("db.statement", "SELECT sql-secret"), attribute.String("url.full", "https://example.test?token=url-secret"),
	)
	span.AddEvent("request", trace.WithAttributes(attribute.String("http.request.headers", "header-secret")))
	span.SetStatus(codes.Error, "status-secret")
	span.End()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	spans := memory.GetSpans()
	if len(spans) != 1 || spans[0].Name != "HTTP GET /users/{id}" || spans[0].Status.Description != "" {
		t.Fatalf("sanitized span = %+v", spans)
	}
	joined := spans[0].Name
	for _, item := range spans[0].Attributes {
		joined += " " + string(item.Key) + "=" + item.Value.Emit()
	}
	for _, item := range spans[0].Resource.Attributes() {
		joined += " " + string(item.Key) + "=" + item.Value.Emit()
	}
	for _, secret := range []string{"path-secret", "sql-secret", "url-secret", "header-secret", "resource-secret", "status-secret"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("span leaked %q: %s", secret, joined)
		}
	}
}

func TestNewProviderRequiresExplicitIdentityAndExporter(t *testing.T) {
	if _, err := NewProvider(context.Background(), Config{}); err == nil {
		t.Fatal("empty config accepted")
	}
	if _, err := NewProvider(context.Background(), Config{ServiceName: "api"}); err == nil {
		t.Fatal("nil exporter accepted")
	}
	provider, err := NewProvider(context.Background(), Config{ServiceName: "api", Exporter: tracetest.NewInMemoryExporter()})
	if err != nil {
		t.Fatal(err)
	}
	_ = provider.Shutdown(context.Background())
}

func TestHTTPClientPropagatesTraceContextWithoutStacking(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})
	var traceparent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceparent = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	ctx, span := provider.Tracer("test").Start(context.Background(), "parent")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	client := HTTPClient(HTTPClient(&http.Client{}))
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	span.End()
	if traceparent == "" || len(recorder.Ended()) != 2 {
		t.Fatalf("traceparent=%q spans=%d", traceparent, len(recorder.Ended()))
	}
}
