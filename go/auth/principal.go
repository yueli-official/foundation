package auth

import (
	"slices"
	"strings"
	"time"
)

// Principal is the application-facing identity from a verified access token.
// Scopes and Roles are always non-nil. Framework adapters may store a Principal
// in request context, but this core package does not own that transport policy.
type Principal struct {
	Subject   string
	ClientID  string
	Issuer    string
	Audience  []string
	Scopes    []string
	Roles     []string
	IssuedAt  time.Time
	ExpiresAt time.Time
	claims    map[string]any
}

// HasScope reports whether the access token grants scope.
func (principal *Principal) HasScope(scope string) bool {
	return principal != nil && slices.Contains(principal.Scopes, scope)
}

// HasRole reports whether the access token carries role.
func (principal *Principal) HasRole(role string) bool {
	return principal != nil && slices.Contains(principal.Roles, role)
}

// ActorKey prefers a delegated human subject and falls back to the OAuth client.
func (principal *Principal) ActorKey() string {
	if principal == nil {
		return ""
	}
	if subject := strings.TrimSpace(principal.Subject); subject != "" {
		return subject
	}
	return strings.TrimSpace(principal.ClientID)
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
