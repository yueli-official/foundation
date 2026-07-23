package urllifecycle

import "fmt"

type ErrorKind string

const (
	ErrorInvalidInput        ErrorKind = "invalid_input"
	ErrorNotFound            ErrorKind = "not_found"
	ErrorConflict            ErrorKind = "conflict"
	ErrorStaleRevision       ErrorKind = "stale_revision"
	ErrorIdempotencyConflict ErrorKind = "idempotency_conflict"
	ErrorCycle               ErrorKind = "cycle"
	ErrorDanglingTarget      ErrorKind = "dangling_target"
	ErrorExternalForbidden   ErrorKind = "external_forbidden"
	ErrorLimitExceeded       ErrorKind = "limit_exceeded"
	ErrorIncompatibleArchive ErrorKind = "incompatible_archive"
	ErrorCorruptState        ErrorKind = "corrupt_state"
	ErrorUnavailable         ErrorKind = "unavailable"
)

type Error struct {
	Kind        ErrorKind
	Field       string
	Message     string
	Diagnostics []Diagnostic
	Cause       error
}

func (err *Error) Error() string {
	if err == nil {
		return ""
	}
	if err.Field != "" {
		return fmt.Sprintf("urllifecycle: %s: %s", err.Field, err.Message)
	}
	return "urllifecycle: " + err.Message
}

func (err *Error) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func invalid(field, format string, args ...any) error {
	return &Error{Kind: ErrorInvalidInput, Field: field, Message: fmt.Sprintf(format, args...)}
}

func conflict(field, format string, args ...any) error {
	return &Error{Kind: ErrorConflict, Field: field, Message: fmt.Sprintf(format, args...)}
}
