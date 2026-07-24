package privacy

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	ErrorInvalidInput      ErrorKind = "invalid_input"
	ErrorDefinitionDrift   ErrorKind = "definition_drift"
	ErrorNotFound          ErrorKind = "not_found"
	ErrorConflict          ErrorKind = "conflict"
	ErrorProtocolViolation ErrorKind = "protocol_violation"
	ErrorStoreUnavailable  ErrorKind = "store_unavailable"
	ErrorOwnerUnavailable  ErrorKind = "owner_unavailable"
)

type Error struct {
	Kind      ErrorKind
	Field     string
	Message   string
	Retryable bool
	Cause     error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Field == "" {
		return "privacy: " + e.Message
	}
	return fmt.Sprintf("privacy: %s: %s", e.Field, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func IsKind(err error, kind ErrorKind) bool {
	var value *Error
	return errors.As(err, &value) && value.Kind == kind
}

func invalid(field, message string) error {
	return &Error{Kind: ErrorInvalidInput, Field: field, Message: message}
}

func conflict(field, message string) error {
	return &Error{Kind: ErrorConflict, Field: field, Message: message}
}
