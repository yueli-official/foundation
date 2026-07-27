// Package ghttpx adapts Platform errors to the Foundation HTTP contract.
// Bind Middleware once per server (s.Use); controllers then use struct-based
// handlers returning (result, error) on success, or r.SetError(errs.New(...))
// on failure.
package ghttpx

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/gogf/gf/v2/util/gvalid"
	foundationhttp "github.com/yueli-official/foundation/go/goframe/http"
	"github.com/yueli-official/foundation/go/problem"

	gerrs "platform/gokit/errs"
	"platform/gokit/log"
)

const traceHeader = "X-Trace-Id"

var problemWriter = foundationhttp.MustWriter(foundationhttp.WriterOptions{TraceHeader: traceHeader})

// Middleware injects a trace id, writes raw success DTOs, and maps failures to
// the Foundation Problem contract. It is the unmetered compatibility entry.
func Middleware(r *ghttp.Request) {
	middleware(nil, nil, r)
}

type ClientKey func(*ghttp.Request) string

// ForwardedClientIPKey opts into GoFrame's forwarded-client-IP resolution.
// The application must configure trusted proxies before choosing this policy.
func ForwardedClientIPKey(request *ghttp.Request) string { return request.GetClientIp() }

// NewMiddleware binds an explicitly owned limiter and client-key topology to
// the response middleware.
func NewMiddleware(limiter *RateLimiter, clientKey ClientKey) func(*ghttp.Request) {
	if limiter == nil || clientKey == nil {
		panic("ghttpx.NewMiddleware requires limiter and client key policy")
	}
	return func(request *ghttp.Request) { middleware(limiter, clientKey, request) }
}

// MediaMiddleware keeps Platform trace/error handling for cacheable public
// media responses without consuming the JSON API's per-client request bucket.
// Media delivery must be protected by CDN/edge bandwidth and abuse controls;
// applying the API request count to every responsive image rendition causes a
// single image-heavy page (or many users behind one NAT) to fail mid-render.
func MediaMiddleware(r *ghttp.Request) {
	middleware(nil, nil, r)
}

func middleware(limiter *RateLimiter, clientKey ClientKey, r *ghttp.Request) {
	traceID := ensureTraceID(r)
	r.SetCtx(log.WithTrace(r.Context(), traceID))
	if limiter != nil && !applyRateLimit(limiter, clientKey, r) {
		writeProblem(r, gerrs.CommonRateLimited, http.StatusTooManyRequests, nil, nil, traceID)
		return
	}

	r.Middleware.Next()

	// If the handler already wrote raw bytes, leave it untouched.
	if r.Response.BufferLength() > 0 {
		return
	}

	if err := r.GetError(); err != nil {
		var c *gerrs.Coded
		switch {
		case errors.As(err, &c):
			params := problem.Parameters(c.Params)
			var violations []problem.Violation
			if c.Code == gerrs.CommonValidationFailed {
				params, violations = validationPayload(c.Params)
			}
			writeProblem(r, c.Code, gerrs.Status(c.Code), params, violations, traceID)
		case gerror.Code(err) == gcode.CodeValidationFailed:
			writeProblem(r, gerrs.CommonValidationFailed, http.StatusBadRequest, nil, goFrameValidationDetails(err), traceID)
		default:
			// Unknown error: log server-side, return a generic message (no internal leak).
			g.Log().Errorf(r.Context(), "unhandled error [trace=%s]: %+v", traceID, err)
			writeProblem(r, gerrs.CommonInternal, http.StatusInternalServerError, nil, nil, traceID)
		}
		return
	}

	r.Response.WriteJson(r.GetHandlerResponse())
}

// RawMediaMiddleware is the raw-byte counterpart to MediaMiddleware. It adds a
// trace id but deliberately leaves cacheable public bytes outside the JSON API
// request bucket. It must not be used for upload, signed grant, or control APIs.
func RawMediaMiddleware(r *ghttp.Request) {
	traceID := ensureTraceID(r)
	r.SetCtx(log.WithTrace(r.Context(), traceID))
	r.Middleware.Next()
}

func validationPayload(params map[string]any) (problem.Parameters, []problem.Violation) {
	if len(params) == 0 {
		return nil, nil
	}
	remaining := make(map[string]any, len(params)-1)
	for key, value := range params {
		if key != "details" {
			remaining[key] = value
		}
	}
	if len(remaining) == 0 {
		remaining = nil
	}
	details, _ := params["details"].([]problem.Violation)
	return remaining, details
}

func goFrameValidationDetails(err error) []problem.Violation {
	var validation gvalid.Error
	if !errors.As(err, &validation) {
		return nil
	}
	details := make([]problem.Violation, 0)
	for _, item := range validation.Items() {
		for field, rules := range item {
			for rule := range rules {
				name := lowerFirst(strings.SplitN(field, "@", 2)[0])
				details = append(details, problem.Violation{
					Pointer: "/" + escapeJSONPointer(name),
					Code:    "validation." + strings.ToLower(rule),
				})
			}
		}
	}
	return details
}

func escapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func writeProblem(r *ghttp.Request, code string, status int, params problem.Parameters, violations []problem.Violation, traceID string) {
	kind, kindErr := problem.NewKind(code, status)
	if kindErr == nil {
		value, valueErr := problem.New(kind, "https://errors.yueli.dev/problems/"+code, traceID, params, violations...)
		if valueErr == nil {
			if writeErr := problemWriter.Write(r, value); writeErr == nil {
				return
			}
		}
	}
	g.Log().Errorf(r.Context(), "invalid public error mapping [trace=%s code=%s status=%d]", traceID, code, status)
	fallback, _ := problem.New(problem.MustKind(gerrs.CommonInternal, http.StatusInternalServerError), "https://errors.yueli.dev/problems/common.internal", traceID, nil)
	if err := problemWriter.Write(r, fallback); err != nil {
		r.Response.ClearBuffer()
		r.Response.WriteStatus(http.StatusInternalServerError)
	}
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToLower(value[:1]) + value[1:]
}

// NewRawRateLimitMiddleware protects RFC/OAuth handlers that use their own
// response contract. It returns the OAuth temporarily_unavailable shape on 429.
func NewRawRateLimitMiddleware(limiter *RateLimiter, clientKey ClientKey) func(*ghttp.Request) {
	if limiter == nil || clientKey == nil {
		panic("ghttpx.NewRawRateLimitMiddleware requires limiter and client key policy")
	}
	return func(request *ghttp.Request) { rawRateLimitMiddleware(limiter, clientKey, request) }
}

func rawRateLimitMiddleware(limiter *RateLimiter, clientKey ClientKey, r *ghttp.Request) {
	traceID := ensureTraceID(r)
	if applyRateLimit(limiter, clientKey, r) {
		r.Middleware.Next()
		return
	}
	r.Response.WriteHeader(http.StatusTooManyRequests)
	r.Response.WriteJson(map[string]string{
		"error":             "temporarily_unavailable",
		"error_description": "rate limit exceeded",
		"trace_id":          traceID,
	})
}

func ensureTraceID(r *ghttp.Request) string {
	traceID := r.Header.Get(traceHeader)
	if traceID == "" {
		traceID = guid.S()
	}
	r.Response.Header().Set(traceHeader, traceID)
	return traceID
}

func applyRateLimit(limiter *RateLimiter, clientKey ClientKey, r *ghttp.Request) bool {
	decision := limiter.Evaluate(clientKey(r))
	for key, value := range decision.Headers() {
		r.Response.Header().Set(key, value)
	}
	return decision.Allowed
}
