// Package identifier owns Yueli's stable identifier formats. It delegates UUID
// construction to a maintained RFC implementation and keeps product callers
// independent from algorithm and encoding choices.
package identifier

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/google/uuid"
)

// UUID remains assignment-compatible with google/uuid so PostgreSQL, JSON and
// text adapters keep using that library's mature implementation.
type UUID = uuid.UUID

var (
	ErrInvalidUUID = errors.New("identifier: invalid canonical UUID")
	uuidPattern    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

// New creates the organization-default entity identifier: an RFC 9562 UUIDv7.
// UUIDv7 improves index locality but is not a strict business sequence and is
// never an authorization secret.
func New() (UUID, error) {
	value, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("identifier: create UUIDv7: %w", err)
	}
	return value, nil
}

// MustNew is for construction paths that cannot recover from loss of the
// operating system clock or cryptographic random source. It panics on failure.
func MustNew() UUID {
	return uuid.Must(New())
}

// Parse accepts only the canonical lowercase hyphenated UUID representation.
// It can read any RFC UUID version; all identifiers newly issued by New are v7.
func Parse(text string) (UUID, error) {
	if !uuidPattern.MatchString(text) {
		return uuid.Nil, ErrInvalidUUID
	}
	value, err := uuid.Parse(text)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %v", ErrInvalidUUID, err)
	}
	return value, nil
}

// Derive creates a deterministic RFC UUIDv5. The owning domain must version
// the namespace and canonical byte representation of name.
func Derive(namespace UUID, canonicalName []byte) UUID {
	return uuid.NewSHA1(namespace, canonicalName)
}
