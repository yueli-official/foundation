// Package auth adapts the framework-independent auth Verifier and Principal to
// GoFrame middleware. It renders failures through the shared Problem writer.
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/gogf/gf/v2/net/ghttp"
	coreauth "github.com/yueli-official/foundation/go/auth"
	goframehttp "github.com/yueli-official/foundation/go/goframe/http"
	"github.com/yueli-official/foundation/go/problem"
)

// TokenVerifier is satisfied by auth.Verifier and keeps middleware tests and
// alternate verifier implementations behind the smallest useful Interface.
type TokenVerifier interface {
	Verify(ctx context.Context, raw string) (*coreauth.Principal, error)
}

// Options configures required and optional-auth middleware.
type Options struct {
	Verifier         TokenVerifier
	Writer           *goframehttp.Writer
	UnauthorizedKind problem.Kind
	UnauthorizedType string
	// TraceID returns the already-established request trace ID.
	TraceID func(*ghttp.Request) string
	// Realm is optional and must contain visible characters excluding quote and backslash.
	Realm string
}

// Middleware owns validated, immutable middleware dependencies.
type Middleware struct {
	verifier         TokenVerifier
	writer           *goframehttp.Writer
	unauthorizedKind problem.Kind
	unauthorizedType string
	traceID          func(*ghttp.Request) string
	realm            string
}

// NewMiddleware validates configuration. UnauthorizedKind must carry HTTP 401.
func NewMiddleware(options Options) (*Middleware, error) {
	if options.Verifier == nil {
		return nil, errors.New("goframe/auth: Verifier is required")
	}
	if options.Writer == nil {
		return nil, errors.New("goframe/auth: Writer is required")
	}
	if options.UnauthorizedKind.Status() != http.StatusUnauthorized {
		return nil, errors.New("goframe/auth: UnauthorizedKind must use HTTP 401")
	}
	if options.TraceID == nil {
		return nil, errors.New("goframe/auth: TraceID is required")
	}
	if _, err := problem.New(options.UnauthorizedKind, options.UnauthorizedType, "configuration-check", nil); err != nil {
		return nil, err
	}
	if !validRealm(options.Realm) {
		return nil, errors.New("goframe/auth: Realm contains invalid characters")
	}
	return &Middleware{
		verifier:         options.Verifier,
		writer:           options.Writer,
		unauthorizedKind: options.UnauthorizedKind,
		unauthorizedType: options.UnauthorizedType,
		traceID:          options.TraceID,
		realm:            options.Realm,
	}, nil
}

// Required requires exactly one Bearer credential and rejects invalid tokens.
func (middleware *Middleware) Required(request *ghttp.Request) {
	raw, state := bearerCredential(request.Request.Header.Values("Authorization"))
	if state != credentialPresent {
		middleware.reject(request, state == credentialMalformed)
		return
	}
	principal, err := middleware.verifier.Verify(request.Context(), raw)
	if err != nil || principal == nil {
		middleware.reject(request, true)
		return
	}
	request.SetCtx(coreauth.NewContext(request.Context(), principal))
	request.Middleware.Next()
}

// Optional permits a missing credential, but rejects a malformed or invalid
// credential instead of silently downgrading an authenticated attempt.
func (middleware *Middleware) Optional(request *ghttp.Request) {
	raw, state := bearerCredential(request.Request.Header.Values("Authorization"))
	if state == credentialMissing {
		request.Middleware.Next()
		return
	}
	if state != credentialPresent {
		middleware.reject(request, true)
		return
	}
	principal, err := middleware.verifier.Verify(request.Context(), raw)
	if err != nil || principal == nil {
		middleware.reject(request, true)
		return
	}
	request.SetCtx(coreauth.NewContext(request.Context(), principal))
	request.Middleware.Next()
}

func (middleware *Middleware) reject(request *ghttp.Request, invalid bool) {
	challenge := "Bearer"
	if middleware.realm != "" {
		challenge += ` realm="` + middleware.realm + `"`
	}
	if invalid {
		challenge += ` error="invalid_token"`
	}
	request.Response.Header().Set("WWW-Authenticate", challenge)
	value, err := problem.New(
		middleware.unauthorizedKind,
		middleware.unauthorizedType,
		middleware.traceID(request),
		nil,
	)
	if err != nil || middleware.writer.Write(request, value) != nil {
		request.Response.ClearBuffer()
		request.Response.Header().Del("Content-Type")
		request.Response.Header().Del("WWW-Authenticate")
		request.Response.WriteStatus(http.StatusInternalServerError)
	}
}

type credentialState uint8

const (
	credentialMissing credentialState = iota
	credentialMalformed
	credentialPresent
)

func bearerCredential(values []string) (string, credentialState) {
	if len(values) == 0 {
		return "", credentialMissing
	}
	if len(values) != 1 {
		return "", credentialMalformed
	}
	fields := strings.Fields(values[0])
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || fields[1] == "" {
		return "", credentialMalformed
	}
	return fields[1], credentialPresent
}

func validRealm(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) || character == '"' || character == '\\' {
			return false
		}
	}
	return true
}
