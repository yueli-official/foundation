package audit

import "fmt"

type ErrorKind string

const (
	ErrorInvalidDefinition   ErrorKind = "invalid_definition"
	ErrorUnknownAction       ErrorKind = "unknown_action"
	ErrorInvalidAttempt      ErrorKind = "invalid_attempt"
	ErrorRejectedEvidence    ErrorKind = "rejected_evidence"
	ErrorTransactionRequired ErrorKind = "transaction_required"
	ErrorIdempotencyConflict ErrorKind = "idempotency_conflict"
	ErrorUnavailable         ErrorKind = "unavailable"
	ErrorCapacity            ErrorKind = "capacity"
	ErrorIntegrityMismatch   ErrorKind = "integrity_mismatch"
	ErrorInvalidCursor       ErrorKind = "invalid_cursor"
	ErrorExportFailed        ErrorKind = "export_failed"
	ErrorArchiveRequired     ErrorKind = "archive_required"
	ErrorHoldConflict        ErrorKind = "hold_conflict"
)

type Error struct {
	Kind    ErrorKind
	Field   string
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Field != "" {
		return fmt.Sprintf("audit: %s: %s", e.Field, e.Message)
	}
	return "audit: " + e.Message
}

func IsKind(err error, kind ErrorKind) bool {
	value, ok := err.(*Error)
	return ok && value.Kind == kind
}

func invalidDefinition(field, message string) error {
	return &Error{Kind: ErrorInvalidDefinition, Field: field, Message: message}
}

func invalidAttempt(field, message string) error {
	return &Error{Kind: ErrorInvalidAttempt, Field: field, Message: message}
}

func rejectedEvidence(field, message string) error {
	return &Error{Kind: ErrorRejectedEvidence, Field: field, Message: message}
}
