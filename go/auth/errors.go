// Package auth verifies signed JWT access tokens independently of HTTP and
// framework middleware. Remote key transport belongs to package jwks.
package auth

import "errors"

var (
	ErrMalformedToken       = errors.New("auth: malformed token")
	ErrTokenTooLarge        = errors.New("auth: token too large")
	ErrUnsupportedAlgorithm = errors.New("auth: unsupported signing algorithm")
	ErrInvalidType          = errors.New("auth: invalid token type")
	ErrMissingKeyID         = errors.New("auth: missing signing key ID")
	ErrKeyUnavailable       = errors.New("auth: signing key unavailable")
	ErrBadSignature         = errors.New("auth: invalid token signature")
	ErrInvalidClaims        = errors.New("auth: invalid token claims")
	ErrMissingExpiry        = errors.New("auth: token expiry is required")
	ErrExpired              = errors.New("auth: token expired")
	ErrNotYetValid          = errors.New("auth: token not yet valid")
	ErrInvalidIssuer        = errors.New("auth: invalid issuer")
	ErrInvalidAudience      = errors.New("auth: invalid audience")
	ErrInvalidLifetime      = errors.New("auth: invalid token lifetime")
	ErrMissingActor         = errors.New("auth: token subject or client ID is required")
)
