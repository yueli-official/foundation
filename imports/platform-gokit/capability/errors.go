package capability

import "fmt"

type ErrorCode string

const (
	ErrorRequired          ErrorCode = "required"
	ErrorUnsupported       ErrorCode = "unsupported"
	ErrorInvalid           ErrorCode = "invalid"
	ErrorDuplicate         ErrorCode = "duplicate"
	ErrorUnknownReference  ErrorCode = "unknown_reference"
	ErrorReferenceMismatch ErrorCode = "reference_mismatch"
)

// ContractError is a stable, machine-classifiable manifest validation error.
// Message is diagnostic text for logs and tests; callers should branch on Code.
type ContractError struct {
	Code    ErrorCode
	Message string
}

func (e *ContractError) Error() string {
	return e.Message
}

func contractError(code ErrorCode, format string, args ...any) error {
	return &ContractError{Code: code, Message: fmt.Sprintf(format, args...)}
}
