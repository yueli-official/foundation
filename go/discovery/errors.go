package discovery

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	ErrorConfiguration ErrorKind = "configuration"
	ErrorContract      ErrorKind = "contract"
	ErrorConflict      ErrorKind = "conflict"
	ErrorSource        ErrorKind = "source"
	ErrorCapacity      ErrorKind = "capacity"
	ErrorEncoding      ErrorKind = "encoding"
	ErrorTarget        ErrorKind = "target"
	ErrorCancelled     ErrorKind = "cancelled"
	ErrorUnsupported   ErrorKind = "unsupported"
)

// Error is a stable, transport-neutral Discovery failure.
type Error struct {
	Kind      ErrorKind
	Code      string
	Path      string
	Protocol  string
	Retryable bool
	Message   string
	Cause     error
}

func (err *Error) Error() string {
	if err == nil {
		return ""
	}
	prefix := "discovery"
	if err.Code != "" {
		prefix += ": " + err.Code
	}
	if err.Path != "" {
		prefix += " at " + err.Path
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
	var target *Error
	return errors.As(err, &target) && target.Kind == kind
}

func failure(kind ErrorKind, code, path, format string, args ...any) error {
	return &Error{
		Kind: kind, Code: code, Path: path,
		Message: fmt.Sprintf(format, args...),
	}
}

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type Diagnostic struct {
	Code      string   `json:"code"`
	Severity  Severity `json:"severity"`
	Path      string   `json:"path,omitempty"`
	Protocol  string   `json:"protocol,omitempty"`
	Reference string   `json:"reference,omitempty"`
	Message   string   `json:"message"`
}

type Report struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
}
