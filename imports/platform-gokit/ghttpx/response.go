// Package ghttpx adapts gokit's envelope/error model to the GoFrame HTTP server.
// Bind Middleware once per server (s.Use); controllers then use struct-based
// handlers returning (result, error) on success, or r.SetError(errs.New(...))
// on failure.
package ghttpx

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/gogf/gf/v2/util/gvalid"

	gerrs "platform/gokit/errs"
	"platform/gokit/log"
	"platform/gokit/response"
)

const traceHeader = "X-Trace-Id"

var defaultRateLimiter = rateLimiterFromEnvironment()

// Middleware injects a trace id, then wraps the handler result in the platform
// envelope with a real HTTP status.
func Middleware(r *ghttp.Request) {
	middleware(defaultRateLimiter, r)
}

func middleware(limiter *RateLimiter, r *ghttp.Request) {
	traceID := ensureTraceID(r)
	r.SetCtx(log.WithTrace(r.Context(), traceID))
	if !applyRateLimit(limiter, r) {
		env := response.Fail(gerrs.CommonRateLimited, "rate limit exceeded", nil)
		env.TraceID = traceID
		r.Response.WriteHeader(http.StatusTooManyRequests)
		r.Response.WriteJson(env)
		return
	}

	r.Middleware.Next()

	// If the handler already wrote raw bytes, leave it untouched.
	if r.Response.BufferLength() > 0 {
		return
	}

	if err := r.GetError(); err != nil {
		var c *gerrs.Coded
		var env response.Envelope
		var status int
		switch {
		case errors.As(err, &c):
			env, status = response.Fail(c.Code, c.Message, c.Params), gerrs.Status(c.Code)
			if c.Code == gerrs.CommonValidationFailed {
				env.Params, env.Details = validationPayload(c.Params)
			}
		case gerror.Code(err) == gcode.CodeValidationFailed:
			env, status = response.Fail(gerrs.CommonValidationFailed, err.Error(), nil), http.StatusBadRequest
			env.Details = goFrameValidationDetails(err)
		default:
			// Unknown error: log server-side, return a generic message (no internal leak).
			g.Log().Errorf(r.Context(), "unhandled error [trace=%s]: %+v", traceID, err)
			env, status = response.Fail(gerrs.CommonInternal, "internal error", nil), http.StatusInternalServerError
		}
		env.TraceID = traceID
		r.Response.ClearBuffer()
		r.Response.WriteHeader(status)
		r.Response.WriteJson(env)
		return
	}

	env := response.OK(r.GetHandlerResponse())
	env.TraceID = traceID
	r.Response.WriteJson(env)
}

func validationPayload(params map[string]any) (map[string]any, []response.ValidationDetail) {
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
	details, _ := params["details"].([]response.ValidationDetail)
	return remaining, details
}

func goFrameValidationDetails(err error) []response.ValidationDetail {
	var validation gvalid.Error
	if !errors.As(err, &validation) {
		return nil
	}
	details := make([]response.ValidationDetail, 0)
	for _, item := range validation.Items() {
		for field, rules := range item {
			for rule := range rules {
				details = append(details, response.ValidationDetail{
					Field: lowerFirst(strings.SplitN(field, "@", 2)[0]),
					Code:  rule,
				})
			}
		}
	}
	return details
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToLower(value[:1]) + value[1:]
}

// RawRateLimitMiddleware protects RFC/OAuth handlers that must not use the
// platform envelope. It returns the OAuth temporarily_unavailable shape on 429.
func RawRateLimitMiddleware(r *ghttp.Request) {
	rawRateLimitMiddleware(defaultRateLimiter, r)
}

func rawRateLimitMiddleware(limiter *RateLimiter, r *ghttp.Request) {
	traceID := ensureTraceID(r)
	if applyRateLimit(limiter, r) {
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

func applyRateLimit(limiter *RateLimiter, r *ghttp.Request) bool {
	allowed, remaining, reset := limiter.Allow(r.GetClientIp())
	resetAfter := max(1, int(math.Ceil(time.Until(reset).Seconds())))
	if remaining >= 0 {
		r.Response.Header().Set("RateLimit-Limit", strconv.Itoa(limiter.limit))
		r.Response.Header().Set("RateLimit-Remaining", strconv.Itoa(remaining))
		r.Response.Header().Set("RateLimit-Reset", strconv.Itoa(resetAfter))
	}
	if !allowed {
		r.Response.Header().Set("Retry-After", strconv.Itoa(resetAfter))
	}
	return allowed
}
