// Package api owns the GoFrame request lifecycle for raw success DTOs and the
// Foundation Problem failure contract.
package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/gogf/gf/v2/util/gvalid"
	foundationhttp "github.com/yueli-official/foundation/go/goframe/http"
	"github.com/yueli-official/foundation/go/goframe/ratelimit"
	"github.com/yueli-official/foundation/go/problem"
)

const defaultTraceHeader = "X-Trace-Id"

type ClientKey func(*ghttp.Request) string

type Options struct {
	TraceHeader string
	Limiter     *ratelimit.Limiter
	ClientKey   ClientKey
	RateLimited problem.Descriptor
	Validation  problem.Descriptor
	Internal    problem.Descriptor
}

type Middleware struct {
	traceHeader string
	limiter     *ratelimit.Limiter
	clientKey   ClientKey
	rateLimited problem.Descriptor
	validation  problem.Descriptor
	internal    problem.Descriptor
	writer      foundationhttp.Writer
}

func New(options Options) (*Middleware, error) {
	if (options.Limiter == nil) != (options.ClientKey == nil) {
		return nil, errors.New("goframeapi: Limiter and ClientKey must be configured together")
	}
	for name, descriptor := range map[string]problem.Descriptor{
		"RateLimited": options.RateLimited,
		"Validation":  options.Validation,
		"Internal":    options.Internal,
	} {
		if descriptor.Kind().Code() == "" || descriptor.Type() == "" {
			return nil, fmt.Errorf("goframeapi: %s descriptor is required", name)
		}
	}
	traceHeader := strings.TrimSpace(options.TraceHeader)
	if traceHeader == "" {
		traceHeader = defaultTraceHeader
	}
	writer, err := foundationhttp.NewWriter(foundationhttp.WriterOptions{TraceHeader: traceHeader})
	if err != nil {
		return nil, err
	}
	return &Middleware{
		traceHeader: traceHeader,
		limiter:     options.Limiter,
		clientKey:   options.ClientKey,
		rateLimited: options.RateLimited,
		validation:  options.Validation,
		internal:    options.Internal,
		writer:      writer,
	}, nil
}

// ForwardedClientIPKey opts into GoFrame's forwarded-client-IP resolution.
// Callers must configure trusted proxies before selecting this topology.
func ForwardedClientIPKey(request *ghttp.Request) string {
	return request.GetClientIp()
}

// Handle injects a trace ID, applies an optional caller-owned limiter, leaves
// raw handler output untouched and maps failures to the Problem contract.
func (middleware *Middleware) Handle(request *ghttp.Request) {
	traceID := ensureTraceID(request, middleware.traceHeader)
	if middleware.limiter != nil && !middleware.applyRateLimit(request) {
		middleware.write(request, middleware.rateLimited, traceID, nil, nil)
		return
	}

	request.Middleware.Next()
	if request.Response.BufferLength() > 0 {
		return
	}
	if err := request.GetError(); err != nil {
		if ok, writeErr := middleware.writer.WriteError(request, err, traceID); ok {
			if writeErr != nil {
				middleware.writeFallback(request, traceID, writeErr)
			}
			return
		}
		if gerror.Code(err) == gcode.CodeValidationFailed {
			middleware.write(request, middleware.validation, traceID, nil, goFrameValidationDetails(err))
			return
		}
		g.Log().Errorf(request.Context(), "unhandled error [trace=%s]: %+v", traceID, err)
		middleware.write(request, middleware.internal, traceID, nil, nil)
		return
	}
	request.Response.WriteJson(request.GetHandlerResponse())
}

func (middleware *Middleware) applyRateLimit(request *ghttp.Request) bool {
	decision := middleware.limiter.Evaluate(middleware.clientKey(request))
	for key, value := range decision.Headers() {
		request.Response.Header().Set(key, value)
	}
	return decision.Allowed
}

func (middleware *Middleware) write(request *ghttp.Request, descriptor problem.Descriptor, traceID string, params problem.Parameters, violations []problem.Violation) {
	value, err := problem.New(descriptor.Kind(), descriptor.Type(), traceID, params, violations...)
	if err == nil {
		err = middleware.writer.Write(request, value)
	}
	if err != nil {
		middleware.writeFallback(request, traceID, err)
	}
}

func (middleware *Middleware) writeFallback(request *ghttp.Request, traceID string, cause error) {
	g.Log().Errorf(request.Context(), "invalid public error mapping [trace=%s]: %+v", traceID, cause)
	value, err := problem.New(middleware.internal.Kind(), middleware.internal.Type(), traceID, nil)
	if err == nil {
		err = middleware.writer.Write(request, value)
	}
	if err != nil {
		request.Response.ClearBuffer()
		request.Response.WriteStatus(http.StatusInternalServerError)
	}
}

func ensureTraceID(request *ghttp.Request, header string) string {
	traceID := request.Header.Get(header)
	if traceID == "" {
		traceID = guid.S()
	}
	request.Response.Header().Set(header, traceID)
	return traceID
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

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToLower(value[:1]) + value[1:]
}
