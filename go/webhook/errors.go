package webhook

import (
	"errors"
	"fmt"
	"time"
)

type ErrorCode string

const (
	ErrorInvalidDefinition ErrorCode = "invalid_definition"
	ErrorInvalidEvent      ErrorCode = "invalid_event"
	ErrorEventTooLarge     ErrorCode = "event_too_large"
	ErrorIdempotency       ErrorCode = "idempotency_conflict"
	ErrorNotFound          ErrorCode = "not_found"
	ErrorStateConflict     ErrorCode = "state_conflict"
	ErrorETagConflict      ErrorCode = "etag_conflict"
	ErrorEndpointUnsafe    ErrorCode = "endpoint_unsafe"
	ErrorSecretUnavailable ErrorCode = "secret_unavailable"
	ErrorSignatureMissing  ErrorCode = "signature_missing"
	ErrorSignatureInvalid  ErrorCode = "signature_invalid"
	ErrorTimestampWindow   ErrorCode = "timestamp_outside_window"
	ErrorEnvelopeInvalid   ErrorCode = "envelope_invalid"
	ErrorTypeForbidden     ErrorCode = "type_forbidden"
	ErrorLimitExceeded     ErrorCode = "limit_exceeded"
	ErrorUnavailable       ErrorCode = "unavailable"
)

type Error struct {
	Code       ErrorCode
	Field      string
	Message    string
	Retryable  bool
	RetryAfter time.Duration
	Cause      error
}

func (err *Error) Error() string {
	if err == nil {
		return ""
	}
	if err.Field != "" {
		return fmt.Sprintf("webhook: %s: %s: %s", err.Code, err.Field, err.Message)
	}
	return fmt.Sprintf("webhook: %s: %s", err.Code, err.Message)
}

func (err *Error) Unwrap() error { return err.Cause }

func IsCode(err error, code ErrorCode) bool {
	var typed *Error
	return errors.As(err, &typed) && typed.Code == code
}

func invalid(code ErrorCode, field, message string) error {
	return &Error{Code: code, Field: field, Message: message}
}

func unavailable(message string, cause error) error {
	return &Error{Code: ErrorUnavailable, Message: message, Retryable: true, Cause: cause}
}
