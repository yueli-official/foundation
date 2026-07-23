package work

import (
	"errors"
	"time"
)

type handlerFailure struct {
	cause      error
	permanent  bool
	retryAfter time.Duration
}

func (err *handlerFailure) Error() string {
	if err == nil || err.cause == nil {
		return ""
	}
	return err.cause.Error()
}

func (err *handlerFailure) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

// Permanent marks a handler failure as non-retryable.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &handlerFailure{cause: err, permanent: true}
}

// RetryAfter overrides the catalog backoff for a retryable handler failure.
func RetryAfter(err error, delay time.Duration) error {
	if err == nil {
		return nil
	}
	return &handlerFailure{cause: err, retryAfter: delay}
}

func classifyFailure(err error) (permanent bool, retryAfter time.Duration) {
	var typed *handlerFailure
	if errors.As(err, &typed) {
		return typed.permanent, typed.retryAfter
	}
	return false, 0
}
