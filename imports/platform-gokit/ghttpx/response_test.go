package ghttpx_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/test/gtest"
	"github.com/yueli-official/foundation/go/problem"

	"platform/gokit/errs"
	"platform/gokit/ghttpx"
)

// ---------------------------------------------------------------------------
// Struct-based request/response types for the success-path handler.
// GoFrame populates r.handlerResponse when a handler returns (result, error).
// ---------------------------------------------------------------------------

type successReq struct {
	g.Meta `path:"/success" method:"get"`
}

type successRes struct {
	X int `json:"x"`
}

type cSuccess struct{}

func (c *cSuccess) Success(_ context.Context, _ *successReq) (*successRes, error) {
	return &successRes{X: 1}, nil
}

// ---------------------------------------------------------------------------
// Error-path handler: plain func using r.SetError.
// ---------------------------------------------------------------------------

var emailTakenCode = errs.Register("identity.email_taken", 409)

func errorHandler(r *ghttp.Request) {
	r.SetError(errs.New(emailTakenCode, "taken", map[string]any{"email": "a@b"}))
}

func codedValidationHandler(r *ghttp.Request) {
	r.SetError(errs.New(errs.CommonValidationFailed, "validation failed", map[string]any{
		"details": []problem.Violation{{Pointer: "/url", Code: "validation.absolute_http_url"}},
	}))
}

type validationReq struct {
	g.Meta `path:"/validation" method:"post"`
	Title  string `json:"title" v:"required"`
}

type validationRes struct{}
type validationController struct{}

func (*validationController) Validate(context.Context, *validationReq) (*validationRes, error) {
	return &validationRes{}, nil
}

// ---------------------------------------------------------------------------
// Helper: newServer starts a g.Server with ghttpx.Middleware and the given
// handler(s) bound to a route group.
// ---------------------------------------------------------------------------

func newSuccessServer(t *gtest.T) *ghttp.Server {
	s := g.Server(t.Name() + "-success")
	s.SetAddr("127.0.0.1:0") // loopback-only: avoids the Windows Firewall prompt
	s.Use(ghttpx.Middleware)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Bind(&cSuccess{})
	})
	s.SetDumpRouterMap(false)
	s.Start()
	return s
}

func newErrorServer(t *gtest.T) *ghttp.Server {
	s := g.Server(t.Name() + "-error")
	s.SetAddr("127.0.0.1:0") // loopback-only: avoids the Windows Firewall prompt
	s.Use(ghttpx.Middleware)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.GET("/error", errorHandler)
	})
	s.SetDumpRouterMap(false)
	s.Start()
	return s
}

func newValidationServer(t *gtest.T) *ghttp.Server {
	s := g.Server(t.Name() + "-validation")
	s.SetAddr("127.0.0.1:0")
	s.Use(ghttpx.Middleware)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Bind(&validationController{})
		group.GET("/coded-validation", codedValidationHandler)
	})
	s.SetDumpRouterMap(false)
	s.Start()
	return s
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestMiddleware_SuccessPath verifies that success is the raw DTO and tracing
// remains in the response header rather than a Data-any envelope.
func TestMiddleware_SuccessPath(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		s := newSuccessServer(t)
		defer s.Shutdown()

		client := g.Client()
		client.SetPrefix(fmt.Sprintf("http://127.0.0.1:%d", s.GetListenedPort()))

		resp, err := client.Get(context.Background(), "/success")
		t.AssertNil(err)
		defer resp.Close()

		body := resp.ReadAllString()
		j := gjson.New(body)

		t.Assert(j.Get("x").Int(), 1)
		t.Assert(j.Contains("data"), false)
		traceID := resp.Header.Get("X-Trace-Id")
		t.AssertNE(traceID, "")
	})
}

// TestMiddleware_CodedErrorPath verifies that a handler calling r.SetError with
// an errs.Coded produces the correct HTTP status (409), code, and params.
func TestMiddleware_CodedErrorPath(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		s := newErrorServer(t)
		defer s.Shutdown()

		client := g.Client()
		client.SetPrefix(fmt.Sprintf("http://127.0.0.1:%d", s.GetListenedPort()))

		resp, err := client.Get(context.Background(), "/error")
		t.AssertNil(err)
		defer resp.Close()

		t.Assert(resp.StatusCode, 409)

		body := resp.ReadAllString()
		j := gjson.New(body)

		t.Assert(j.Get("code").String(), "identity.email_taken")
		t.Assert(j.Get("params.email").String(), "a@b")
	})
}

func TestMiddleware_ValidationDetailsAreTopLevel(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		s := newValidationServer(t)
		defer s.Shutdown()
		client := g.Client()
		client.SetPrefix(fmt.Sprintf("http://127.0.0.1:%d", s.GetListenedPort()))

		coded, err := client.Get(context.Background(), "/coded-validation")
		t.AssertNil(err)
		defer coded.Close()
		codedJSON := gjson.New(coded.ReadAllString())
		t.Assert(codedJSON.Get("code").String(), errs.CommonValidationFailed)
		t.Assert(codedJSON.Get("violations.0.pointer").String(), "/url")
		t.Assert(codedJSON.Get("violations.0.code").String(), "validation.absolute_http_url")
		t.Assert(strings.Contains(codedJSON.MustToJsonString(), `"params"`), false)

		builtIn, err := client.ContentJson().Post(context.Background(), "/validation", `{}`)
		t.AssertNil(err)
		defer builtIn.Close()
		builtInJSON := gjson.New(builtIn.ReadAllString())
		t.Assert(builtInJSON.Get("code").String(), errs.CommonValidationFailed)
		t.Assert(builtInJSON.Get("violations.0.pointer").String(), "/title")
		t.Assert(builtInJSON.Get("violations.0.code").String(), "validation.required")
	})
}

// ---------------------------------------------------------------------------
// Internal error leak test: a plain (non-Coded) error must NOT be surfaced
// to the client. The response must be generic ("internal error") and the raw
// error detail ("secret db detail xyz") must never appear in the body.
// ---------------------------------------------------------------------------

func internalLeakHandler(r *ghttp.Request) {
	r.SetError(errors.New("secret db detail xyz"))
}

func newInternalLeakServer(t *gtest.T) *ghttp.Server {
	s := g.Server(t.Name() + "-leak")
	s.SetAddr("127.0.0.1:0") // loopback-only: avoids the Windows Firewall prompt
	s.Use(ghttpx.Middleware)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.GET("/leak", internalLeakHandler)
	})
	s.SetDumpRouterMap(false)
	s.Start()
	return s
}

// TestMiddleware_InternalErrorNotLeaked verifies that a plain (non-Coded) error
// set via r.SetError returns HTTP 500 with code "common.internal" and the
// generic message "internal error" — never the raw error string.
func TestMiddleware_InternalErrorNotLeaked(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		s := newInternalLeakServer(t)
		defer s.Shutdown()

		client := g.Client()
		client.SetPrefix(fmt.Sprintf("http://127.0.0.1:%d", s.GetListenedPort()))

		resp, err := client.Get(context.Background(), "/leak")
		t.AssertNil(err)
		defer resp.Close()

		t.Assert(resp.StatusCode, 500)

		body := resp.ReadAllString()
		j := gjson.New(body)

		t.Assert(j.Get("code").String(), "common.internal")
		t.Assert(resp.Header.Get("Content-Type"), "application/problem+json")
		t.Assert(j.Get("type").String(), "https://errors.yueli.dev/problems/common.internal")
		// The raw internal error detail must never be sent to the client.
		t.Assert(strings.Contains(body, "secret db detail"), false)
	})
}
