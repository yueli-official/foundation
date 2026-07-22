// Package httpclient decodes the Foundation raw-success/Problem HTTP contract.
package httpclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/yueli-official/foundation/go/problem"
)

const (
	defaultSuccessLimit = 1 << 20
	defaultProblemLimit = 64 << 10
)

type Limits struct {
	SuccessBytes int64
	ProblemBytes int64
}

type RemoteError struct {
	Problem problem.Problem
}

func (value *RemoteError) Error() string { return value.Problem.Code }

type ProtocolError struct {
	Code string
}

func (value *ProtocolError) Error() string { return value.Code }

// DecodeJSON accepts a raw JSON success DTO and requires RFC 9457 Problem on
// non-2xx. It never exposes remote diagnostic text as an error message.
func DecodeJSON[T any](response *http.Response, limits Limits) (T, error) {
	var zero T
	if response == nil {
		return zero, &ProtocolError{Code: "foundation.response.nil"}
	}
	successLimit, err := limit(limits.SuccessBytes, defaultSuccessLimit)
	if err != nil {
		return zero, err
	}
	problemLimit, err := limit(limits.ProblemBytes, defaultProblemLimit)
	if err != nil {
		return zero, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return zero, decodeProblem(response, problemLimit)
	}
	if response.StatusCode == http.StatusNoContent || response.Request != nil && response.Request.Method == http.MethodHead {
		return zero, nil
	}
	if !hasMediaType(response.Header.Get("Content-Type"), "application/json") {
		return zero, &ProtocolError{Code: "foundation.response.invalid_content_type"}
	}
	body, err := readLimited(response.Body, successLimit)
	if err != nil {
		return zero, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&zero); err != nil {
		return zero, &ProtocolError{Code: "foundation.response.invalid_json"}
	}
	if err := requireEOF(decoder); err != nil {
		return zero, err
	}
	return zero, nil
}

func decodeProblem(response *http.Response, limit int64) error {
	if !hasMediaType(response.Header.Get("Content-Type"), "application/problem+json") {
		return &ProtocolError{Code: "foundation.problem.invalid_content_type"}
	}
	body, err := readLimited(response.Body, limit)
	if err != nil {
		return err
	}
	value, err := problem.Decode(body)
	if err != nil {
		return &ProtocolError{Code: "foundation.problem.invalid_body"}
	}
	if value.Status != response.StatusCode {
		return &ProtocolError{Code: "foundation.problem.status_mismatch"}
	}
	if traceID := response.Header.Get("X-Trace-Id"); traceID != "" && traceID != value.TraceID {
		return &ProtocolError{Code: "foundation.problem.trace_mismatch"}
	}
	return &RemoteError{Problem: value}
}

func hasMediaType(raw, expected string) bool {
	mediaType, _, err := mime.ParseMediaType(raw)
	return err == nil && strings.EqualFold(mediaType, expected)
}

func limit(configured, fallback int64) (int64, error) {
	if configured == 0 {
		return fallback, nil
	}
	if configured < 0 {
		return 0, &ProtocolError{Code: "foundation.response.invalid_limit"}
	}
	return configured, nil
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	if reader == nil {
		return nil, &ProtocolError{Code: "foundation.response.invalid_body"}
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("foundation.response.read: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, &ProtocolError{Code: "foundation.response.body_too_large"}
	}
	return body, nil
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return &ProtocolError{Code: "foundation.response.trailing_json"}
}
