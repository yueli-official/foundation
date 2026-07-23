package authorization

import (
	"errors"
	"fmt"
)

// ErrorKind is a stable class of authorization failure.
type ErrorKind string

const (
	ErrorInvalidDefinition ErrorKind = "invalid_definition"
	ErrorInvalidInput      ErrorKind = "invalid_input"
	ErrorDenied            ErrorKind = "denied"
	ErrorConflict          ErrorKind = "conflict"
	ErrorNotFound          ErrorKind = "not_found"
	ErrorExpired           ErrorKind = "expired"
	ErrorInvariant         ErrorKind = "invariant_violation"
	ErrorUnavailable       ErrorKind = "unavailable"
)

// Error reports a stable failure kind without exposing Adapter diagnostics.
type Error struct {
	Kind    ErrorKind
	Field   string
	Message string
	err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Field != "" {
		return fmt.Sprintf("authorization: %s: %s", e.Field, e.Message)
	}
	return "authorization: " + e.Message
}

func (e *Error) Unwrap() error { return e.err }

// Is reports whether err is an authorization Error of kind.
func Is(err error, kind ErrorKind) bool {
	var target *Error
	return errors.As(err, &target) && target.Kind == kind
}

// IsInvalidDefinition reports whether Compile rejected a consumer definition.
func IsInvalidDefinition(err error) bool { return Is(err, ErrorInvalidDefinition) }

func invalidDefinition(field, format string, args ...any) error {
	return &Error{Kind: ErrorInvalidDefinition, Field: field, Message: fmt.Sprintf(format, args...)}
}
