package siteprofile

import (
	"errors"
	"fmt"
)

var (
	ErrNotInitialized = errors.New("siteprofile: profile is not initialized")
	ErrCorruptState   = errors.New("siteprofile: corrupt persisted state")
)

type RevisionConflictError struct {
	Expected Revision
	Actual   Revision
}

func (e *RevisionConflictError) Error() string {
	return fmt.Sprintf("siteprofile: revision conflict: expected %d, actual %d", e.Expected, e.Actual)
}

type ValidationError struct {
	Diagnostics []Diagnostic
}

func (e *ValidationError) Error() string {
	if len(e.Diagnostics) == 0 {
		return "siteprofile: profile validation failed"
	}
	return "siteprofile: profile validation failed: " + e.Diagnostics[0].Message
}
