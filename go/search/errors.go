package search

import "fmt"

type ErrorKind string

const (
	ErrorInvalidDefinition     ErrorKind = "invalid_definition"
	ErrorInvalidDocument       ErrorKind = "invalid_document"
	ErrorInvalidQuery          ErrorKind = "invalid_query"
	ErrorUnsupportedCapability ErrorKind = "unsupported_capability"
	ErrorStaleRevision         ErrorKind = "stale_revision"
	ErrorRevisionConflict      ErrorKind = "revision_conflict"
	ErrorIdempotencyConflict   ErrorKind = "idempotency_conflict"
	ErrorTransactionRequired   ErrorKind = "transaction_required"
	ErrorInvalidCursor         ErrorKind = "invalid_cursor"
	ErrorGenerationGone        ErrorKind = "generation_gone"
	ErrorRebuildConflict       ErrorKind = "rebuild_conflict"
	ErrorUnavailable           ErrorKind = "unavailable"
	ErrorCapacity              ErrorKind = "capacity"
)

type Error struct {
	Kind    ErrorKind
	Field   string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("search: %s: %s", e.Kind, e.Message)
	}
	return fmt.Sprintf("search: %s: %s %s", e.Kind, e.Field, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

func IsKind(err error, kind ErrorKind) bool {
	for err != nil {
		if value, ok := err.(*Error); ok {
			return value.Kind == kind
		}
		type unwrapper interface{ Unwrap() error }
		value, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = value.Unwrap()
	}
	return false
}
