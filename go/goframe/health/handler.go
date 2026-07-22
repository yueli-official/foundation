// Package goframehealth adapts the ordinary health Runner to GoFrame.
package goframehealth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/yueli-official/foundation/go/goframe/http"
	"github.com/yueli-official/foundation/go/health"
	"github.com/yueli-official/foundation/go/problem"
)

type HandlerOptions struct {
	Runner   *health.Runner
	NotReady problem.Descriptor
	Writer   goframehttp.Writer
	TraceID  func(*ghttp.Request) string
}

func Handler(options HandlerOptions) (func(*ghttp.Request), error) {
	if options.Runner == nil {
		return nil, errors.New("goframehealth: Runner is required")
	}
	if options.NotReady.Kind().Status() != 503 {
		return nil, errors.New("goframehealth: NotReady status must be 503")
	}
	writer := options.Writer
	if writer == (goframehttp.Writer{}) {
		writer = goframehttp.MustWriter(goframehttp.WriterOptions{})
	}
	return func(request *ghttp.Request) {
		report := options.Runner.Run(request.Context())
		if report.Ready() {
			request.Response.WriteJson(map[string]string{"status": "ready"})
			return
		}
		traceID := ""
		if options.TraceID != nil {
			traceID = options.TraceID(request)
		}
		if traceID == "" {
			traceID = newTraceID()
		}
		mapped, err := problem.NewError(options.NotReady, problem.Parameters{"failed": report.Failed})
		if err != nil {
			request.Response.WriteStatus(500)
			return
		}
		if _, err := writer.WriteError(request, mapped, traceID); err != nil {
			request.Response.WriteStatus(500)
		}
	}, nil
}

func newTraceID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "health-trace-unavailable"
	}
	return hex.EncodeToString(value[:])
}
