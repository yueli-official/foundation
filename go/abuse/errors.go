package abuse

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	ErrorInvalidDefinition     ErrorKind = "invalid_definition"
	ErrorInvalidInput          ErrorKind = "invalid_input"
	ErrorUnknownAction         ErrorKind = "unknown_action"
	ErrorConflict              ErrorKind = "conflict"
	ErrorDefinitionDrift       ErrorKind = "definition_drift"
	ErrorStoreUnavailable      ErrorKind = "store_unavailable"
	ErrorStoreContention       ErrorKind = "store_contention"
	ErrorVerifierUnavailable   ErrorKind = "verifier_unavailable"
	ErrorVerifierConfiguration ErrorKind = "verifier_configuration"
)

// Error is transport-neutral and deliberately never contains raw Signals or
// proof tokens.
type Error struct {
	Kind      ErrorKind
	Field     string
	Message   string
	Retryable bool
	Cause     error
}

func (err *Error) Error() string {
	if err == nil {
		return ""
	}
	prefix := "abuse"
	if err.Field != "" {
		prefix += ": " + err.Field
	}
	if err.Message != "" {
		prefix += ": " + err.Message
	}
	return prefix
}

func (err *Error) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func IsKind(err error, kind ErrorKind) bool {
	var typed *Error
	return errors.As(err, &typed) && typed.Kind == kind
}

func invalidDefinition(field, format string, args ...any) error {
	return &Error{Kind: ErrorInvalidDefinition, Field: field, Message: fmt.Sprintf(format, args...)}
}

func invalidInput(field, format string, args ...any) error {
	return &Error{Kind: ErrorInvalidInput, Field: field, Message: fmt.Sprintf(format, args...)}
}

func conflict(field, format string, args ...any) error {
	return &Error{Kind: ErrorConflict, Field: field, Message: fmt.Sprintf(format, args...)}
}

func unavailable(field, message string, cause error) error {
	return &Error{Kind: ErrorStoreUnavailable, Field: field, Message: message, Retryable: true, Cause: cause}
}
