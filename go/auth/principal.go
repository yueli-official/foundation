package auth

import (
	"slices"
	"strings"
	"time"
)

type SubjectKind string

const (
	SubjectUser   SubjectKind = "user"
	SubjectClient SubjectKind = "client"
	SubjectGuest  SubjectKind = "guest"
)

// Principal is the application-facing identity from a verified access token.
// Scopes and Roles are always non-nil. Framework adapters may store a Principal
// in request context, but this core package does not own that transport policy.
type Principal struct {
	Subject     string
	SubjectKind SubjectKind
	ClientID    string
	Issuer      string
	Audience    []string
	Scopes      []string
	Roles       []string
	IssuedAt    time.Time
	ExpiresAt   time.Time
	claims      map[string]any
}

func (principal *Principal) IsUser() bool {
	return principal != nil && principal.SubjectKind == SubjectUser && strings.TrimSpace(principal.Subject) != ""
}

// HasScope reports whether the access token grants scope.
func (principal *Principal) HasScope(scope string) bool {
	return principal != nil && slices.Contains(principal.Scopes, scope)
}

// HasRole reports whether the access token carries role.
func (principal *Principal) HasRole(role string) bool {
	return principal != nil && slices.Contains(principal.Roles, role)
}

// ActorKey returns the identifier that matches the token's declared actor kind.
func (principal *Principal) ActorKey() string {
	if principal == nil {
		return ""
	}
	switch principal.SubjectKind {
	case SubjectUser, SubjectGuest:
		return strings.TrimSpace(principal.Subject)
	case SubjectClient:
		return strings.TrimSpace(principal.ClientID)
	default:
		return ""
	}
}

// Claim returns a verified raw claim for forward-compatible application use.
// Security decisions should prefer typed Principal fields and verifier policy.
func (principal *Principal) Claim(name string) (any, bool) {
	if principal == nil {
		return nil, false
	}
	value, ok := principal.claims[name]
	return value, ok
}
