// Package goframehttp adapts the framework-independent Problem contract to a
// GoFrame HTTP response. It does not own application error mapping or success
// DTOs.
package goframehttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/yueli-official/foundation/go/problem"
)

const (
	defaultMaxBodyBytes = 64 << 10
	contentType         = "application/problem+json"
	defaultTraceHeader  = "X-Trace-Id"
)

var ErrBodyTooLarge = errors.New("goframehttp: Problem body exceeds configured limit")

var headerNamePattern = regexp.MustCompile("^[!#$%&'*+\\-.^_`|~0-9A-Za-z]+$")

type WriterOptions struct {
	// MaxBodyBytes bounds the serialized response. Zero uses 64 KiB.
	MaxBodyBytes int
	// TraceHeader customizes the response header. Empty uses X-Trace-Id.
	TraceHeader string
}

// Writer is immutable after construction and safe for concurrent handlers.
type Writer struct {
	maxBodyBytes int
	traceHeader  string
}

func NewWriter(options WriterOptions) (Writer, error) {
	maxBodyBytes := options.MaxBodyBytes
	if maxBodyBytes == 0 {
		maxBodyBytes = defaultMaxBodyBytes
	}
	if maxBodyBytes < 1 {
		return Writer{}, errors.New("goframehttp: MaxBodyBytes must be positive")
	}
	traceHeader := options.TraceHeader
	if traceHeader == "" {
		traceHeader = defaultTraceHeader
	}
	if !headerNamePattern.MatchString(traceHeader) {
		return Writer{}, errors.New("goframehttp: TraceHeader is invalid")
	}
	return Writer{maxBodyBytes: maxBodyBytes, traceHeader: traceHeader}, nil
}

func MustWriter(options WriterOptions) Writer {
	writer, err := NewWriter(options)
	if err != nil {
		panic(err)
	}
	return writer
}

// Write validates and serializes the body before mutating the response. The
// status, content type and trace header are derived exclusively from value.
func (writer Writer) Write(request *ghttp.Request, value problem.Problem) error {
	if request == nil {
		return errors.New("goframehttp: request is nil")
	}
	if err := value.Validate(); err != nil {
		return fmt.Errorf("goframehttp: invalid Problem: %w", err)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("goframehttp: encode Problem: %w", err)
	}
	if len(body) > writer.maxBodyBytes {
		return ErrBodyTooLarge
	}

	request.Response.ClearBuffer()
	request.Response.Header().Set("Content-Type", contentType)
	request.Response.Header().Set(writer.traceHeader, value.TraceID)
	request.Response.WriteHeader(value.Status)
	request.Response.Write(body)
	return nil
}
