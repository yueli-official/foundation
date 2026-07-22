// Package telemetry builds privacy-bounded OpenTelemetry providers and HTTP clients.
//
// Exporters supplied to NewProvider are always wrapped by the package's
// sanitizer. Applications retain ownership of exporter transport and
// environment parsing, while Foundation owns the safe export boundary.
package telemetry

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
)

type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Attributes     map[string]string
	Exporter       sdktrace.SpanExporter
	Sampler        sdktrace.Sampler
}

// NewProvider constructs a provider with a mandatory sanitizing export
// boundary. It does not mutate OpenTelemetry globals.
func NewProvider(ctx context.Context, cfg Config) (*sdktrace.TracerProvider, error) {
	if strings.TrimSpace(cfg.ServiceName) == "" {
		return nil, errors.New("telemetry requires service.name")
	}
	if cfg.Exporter == nil {
		return nil, errors.New("telemetry requires an exporter")
	}
	res, err := newResource(ctx, cfg)
	if err != nil {
		return nil, err
	}
	sampler := cfg.Sampler
	if sampler == nil {
		sampler = sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(NewSanitizingExporter(cfg.Exporter)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	), nil
}

// InstallGlobal installs a provider and the W3C trace-context/baggage
// propagator. Global ownership remains explicit at the process entry point.
func InstallGlobal(provider *sdktrace.TracerProvider) error {
	if provider == nil {
		return errors.New("telemetry requires a provider")
	}
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	return nil
}

// HTTPClient clones a client and instruments its transport. Calling it more
// than once does not stack OpenTelemetry transports.
func HTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	clone := *client
	transport := clone.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if _, instrumented := transport.(*otelhttp.Transport); !instrumented {
		clone.Transport = otelhttp.NewTransport(transport)
	}
	return &clone
}

// ShutdownWithTimeout flushes spans on a fresh bounded context.
func ShutdownWithTimeout(shutdown func(context.Context) error, timeout time.Duration) error {
	if shutdown == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return shutdown(ctx)
}

func newResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	attributes := []attribute.KeyValue{semconv.ServiceName(strings.TrimSpace(cfg.ServiceName))}
	if value := strings.TrimSpace(cfg.ServiceVersion); value != "" {
		attributes = append(attributes, semconv.ServiceVersion(value))
	}
	if value := strings.TrimSpace(cfg.Environment); value != "" {
		attributes = append(attributes, semconv.DeploymentEnvironmentName(value))
	}
	keys := make([]string, 0, len(cfg.Attributes))
	for key := range cfg.Attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, rawKey := range keys {
		if key := strings.TrimSpace(rawKey); key != "" {
			attributes = append(attributes, attribute.String(key, cfg.Attributes[rawKey]))
		}
	}
	return resource.New(ctx,
		resource.WithFromEnv(), resource.WithTelemetrySDK(), resource.WithProcess(),
		resource.WithOS(), resource.WithContainer(), resource.WithAttributes(attributes...),
	)
}
