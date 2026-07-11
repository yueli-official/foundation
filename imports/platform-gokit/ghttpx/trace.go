package ghttpx

import (
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TraceRouteMiddleware enriches GoFrame's active server span after routing.
// GoFrame 2.10 changes the span name but does not publish semantic route or
// method attributes; the export privacy boundary needs these trusted values to
// replace the raw URL-path span name without collapsing all routes together.
func TraceRouteMiddleware(r *ghttp.Request) {
	r.Middleware.Next()
	span := trace.SpanFromContext(r.Context())
	if !span.IsRecording() {
		return
	}
	attributes := []attribute.KeyValue{attribute.String("http.request.method", r.Method)}
	if handler := r.GetServeHandler(); handler != nil && handler.Handler.Router != nil {
		if route := strings.TrimSpace(handler.Handler.Router.Uri); route != "" {
			attributes = append(attributes, attribute.String("http.route", route))
		}
	}
	span.SetAttributes(attributes...)
}
