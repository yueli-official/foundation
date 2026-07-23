package traffic

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	ErrorInvalidInput ErrorKind = "invalid_input"
	ErrorConflict     ErrorKind = "conflict"
	ErrorUnavailable  ErrorKind = "unavailable"
)

// Error is a stable, transport-neutral failure.
type Error struct {
	Kind    ErrorKind
	Field   string
	Message string
	Cause   error
}

func (err *Error) Error() string {
	if err == nil {
		return ""
	}
	prefix := "traffic"
	if err.Field != "" {
		prefix += ": " + err.Field
	}
	if err.Message == "" {
		return prefix
	}
	return prefix + ": " + err.Message
}

func (err *Error) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func IsKind(err error, kind ErrorKind) bool {
	var target *Error
	return errors.As(err, &target) && target.Kind == kind
}

func invalid(field, format string, args ...any) error {
	return &Error{Kind: ErrorInvalidInput, Field: field, Message: fmt.Sprintf(format, args...)}
}

func conflict(field, format string, args ...any) error {
	return &Error{Kind: ErrorConflict, Field: field, Message: fmt.Sprintf(format, args...)}
}
