package httpclient

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func response(status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestDecodeJSONRawSuccess(t *testing.T) {
	value, err := DecodeJSON[struct {
		ID string `json:"id"`
	}](response(200, "application/json; charset=utf-8", `{"id":"one"}`), Limits{})
	if err != nil || value.ID != "one" {
		t.Fatalf("DecodeJSON() = %#v, %v", value, err)
	}
}

func TestDecodeJSONProblem(t *testing.T) {
	value := response(409, "application/problem+json", `{"type":"https://errors.example.test/problems/catalog.conflict","status":409,"code":"catalog.conflict","traceId":"trace-conflict"}`)
	value.Header.Set("X-Trace-Id", "trace-conflict")
	_, err := DecodeJSON[any](value, Limits{})
	var remote *RemoteError
	if !errors.As(err, &remote) || remote.Problem.Code != "catalog.conflict" {
		t.Fatalf("error = %#v", err)
	}
}

func TestDecodeJSONRejectsLegacyEnvelopeErrorAndOversizedSuccess(t *testing.T) {
	_, err := DecodeJSON[any](response(400, "application/json", `{"code":"legacy.failed"}`), Limits{})
	var protocol *ProtocolError
	if !errors.As(err, &protocol) || protocol.Code != "foundation.problem.invalid_content_type" {
		t.Fatalf("legacy error = %#v", err)
	}
	_, err = DecodeJSON[any](response(200, "application/json", `{"value":"too large"}`), Limits{SuccessBytes: 8})
	if !errors.As(err, &protocol) || protocol.Code != "foundation.response.body_too_large" {
		t.Fatalf("oversized error = %#v", err)
	}
}
