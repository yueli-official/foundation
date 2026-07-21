package auth

import "context"

type principalContextKey struct{}

// NewContext returns a child context carrying a verified Principal.
func NewContext(parent context.Context, principal *Principal) context.Context {
	if parent == nil {
		panic("auth: nil parent context")
	}
	return context.WithValue(parent, principalContextKey{}, principal)
}

// FromContext returns the verified Principal stored by NewContext.
func FromContext(ctx context.Context) (*Principal, bool) {
	if ctx == nil {
		return nil, false
	}
	principal, ok := ctx.Value(principalContextKey{}).(*Principal)
	return principal, ok && principal != nil
}
