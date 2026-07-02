// Package ghttpx adapts gokit's envelope/error model to the GoFrame HTTP server.
// Bind Middleware once per server (s.Use); controllers then use struct-based
// handlers returning (result, error) on success, or r.SetError(errs.New(...))
// on failure.
package ghttpx

import (
	"errors"
	"net/http"

	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"

	gerrs "platform/gokit/errs"
	"platform/gokit/log"
	"platform/gokit/response"
)

const traceHeader = "X-Trace-Id"

// Middleware injects a trace id, then wraps the handler result in the platform
// envelope with a real HTTP status.
func Middleware(r *ghttp.Request) {
	traceID := r.Header.Get(traceHeader)
	if traceID == "" {
		traceID = guid.S()
	}
	r.SetCtx(log.WithTrace(r.Context(), traceID))
	r.Response.Header().Set(traceHeader, traceID)

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
		case gerror.Code(err) == gcode.CodeValidationFailed:
			env, status = response.Fail(gerrs.CommonValidationFailed, err.Error(), nil), http.StatusBadRequest
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
