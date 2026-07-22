package telemetry

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type sanitizingExporter struct{ next sdktrace.SpanExporter }

// NewSanitizingExporter removes request secrets, SQL, bodies, full URLs and
// diagnostic messages immediately before spans leave the process.
func NewSanitizingExporter(next sdktrace.SpanExporter) sdktrace.SpanExporter {
	return &sanitizingExporter{next: next}
}

func (value *sanitizingExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	clean := make([]sdktrace.ReadOnlySpan, len(spans))
	for index, span := range spans {
		clean[index] = sanitizeSpan(span)
	}
	return value.next.ExportSpans(ctx, clean)
}

func (value *sanitizingExporter) Shutdown(ctx context.Context) error { return value.next.Shutdown(ctx) }

type sanitizedSpan struct {
	sdktrace.ReadOnlySpan
	name       string
	attributes []attribute.KeyValue
	events     []sdktrace.Event
	links      []sdktrace.Link
	resource   *resource.Resource
	status     sdktrace.Status
}

func (value *sanitizedSpan) Name() string                     { return value.name }
func (value *sanitizedSpan) Attributes() []attribute.KeyValue { return value.attributes }
func (value *sanitizedSpan) Events() []sdktrace.Event         { return value.events }
func (value *sanitizedSpan) Links() []sdktrace.Link           { return value.links }
func (value *sanitizedSpan) Resource() *resource.Resource     { return value.resource }
func (value *sanitizedSpan) Status() sdktrace.Status          { return value.status }

func sanitizeSpan(span sdktrace.ReadOnlySpan) sdktrace.ReadOnlySpan {
	attributes := sanitizeAttributes(span.Attributes())
	name := span.Name()
	if span.SpanKind() == trace.SpanKindServer {
		name = safeServerSpanName(attributes)
	}
	events := span.Events()
	cleanEvents := make([]sdktrace.Event, len(events))
	for index, event := range events {
		cleanEvents[index] = event
		cleanEvents[index].Attributes = sanitizeAttributes(event.Attributes)
	}
	links := span.Links()
	cleanLinks := make([]sdktrace.Link, len(links))
	for index, link := range links {
		cleanLinks[index] = link
		cleanLinks[index].Attributes = sanitizeAttributes(link.Attributes)
	}
	cleanResource := resource.Empty()
	if span.Resource() != nil {
		cleanResource = resource.NewWithAttributes(span.Resource().SchemaURL(), sanitizeAttributes(span.Resource().Attributes())...)
	}
	status := span.Status()
	if span.SpanKind() == trace.SpanKindServer {
		status = normalizeHTTPServerStatus(status, attributes)
	}
	status.Description = ""
	return &sanitizedSpan{ReadOnlySpan: span, name: name, attributes: attributes, events: cleanEvents, links: cleanLinks, resource: cleanResource, status: status}
}

func normalizeHTTPServerStatus(status sdktrace.Status, attributes []attribute.KeyValue) sdktrace.Status {
	for _, item := range attributes {
		if string(item.Key) == "http.response.status_code" && item.Value.Type() == attribute.INT64 {
			if item.Value.AsInt64() >= 500 {
				status.Code = codes.Error
			} else {
				status.Code = codes.Unset
			}
			return status
		}
	}
	return status
}

func safeServerSpanName(attributes []attribute.KeyValue) string {
	method, route := "", ""
	for _, item := range attributes {
		switch string(item.Key) {
		case "http.request.method", "http.method":
			method = strings.ToUpper(strings.TrimSpace(item.Value.AsString()))
		case "http.route", "http.request.route":
			route = strings.TrimSpace(item.Value.AsString())
		}
	}
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "CONNECT", "OPTIONS", "TRACE":
	default:
		method = "request"
	}
	if route != "" && strings.HasPrefix(route, "/") && !strings.ContainsAny(route, "?#") && len(route) <= 256 {
		return "HTTP " + method + " " + route
	}
	return "HTTP " + method
}

func sanitizeAttributes(attributes []attribute.KeyValue) []attribute.KeyValue {
	clean := make([]attribute.KeyValue, 0, len(attributes))
	for _, item := range attributes {
		if !sensitiveTelemetryKey(string(item.Key)) {
			clean = append(clean, item)
		}
	}
	return clean
}

func sensitiveTelemetryKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, fragment := range []string{
		"authorization", "cookie", "header", "baggage", "credential", "password", "secret", "token",
		"request.body", "response.body", "db.statement", "db.query.text", "db.operation.parameter",
		"db.link", "url.query", "url.full", "exception.message", "exception.stacktrace", "error.message",
	} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return strings.HasSuffix(key, ".url")
}
