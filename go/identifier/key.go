package identifier

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	base32Alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

	// MaxAllocationAttempts is deliberately fixed across consumers. A caller
	// may not silently weaken or create an unbounded collision policy.
	MaxAllocationAttempts = 8
)

var (
	ErrInvalidKey         = errors.New("identifier: invalid public key")
	ErrInvalidProfile     = errors.New("identifier: invalid key profile")
	ErrEntropyUnavailable = errors.New("identifier: cryptographic entropy unavailable")
	ErrCollisionExhausted = errors.New("identifier: public key collision attempts exhausted")
)

// Key is a canonical public locator. It is safe to log and display, and it
// never conveys authority by possession.
type Key string

func (k Key) String() string { return string(k) }

// KeyProfile identifies an immutable, versioned public-key format.
type KeyProfile uint8

type keyProfileDefinition struct {
	name     string
	alphabet string
	length   int
}

const (
	keyProfileUnknown KeyProfile = iota

	// CompactURLV1 is an eight-character Base58 locator for route-scoped public
	// addresses. It has about 47 bits of space and therefore requires Allocate.
	CompactURLV1

	// HumanCodeV1 is a ten-character, case-sensitive canonical Crockford Base32
	// locator for values that people may transcribe. It is not a bearer token.
	HumanCodeV1

	// OpaquePublicV1 is a 16-character Crockford Base32 public reference with
	// 80 bits of random space. It remains non-secret despite being unpredictable.
	OpaquePublicV1
)

func (p KeyProfile) definition() (keyProfileDefinition, bool) {
	switch p {
	case CompactURLV1:
		return keyProfileDefinition{name: "compact-url-v1", alphabet: base58Alphabet, length: 8}, true
	case HumanCodeV1:
		return keyProfileDefinition{name: "human-code-v1", alphabet: base32Alphabet, length: 10}, true
	case OpaquePublicV1:
		return keyProfileDefinition{name: "opaque-public-v1", alphabet: base32Alphabet, length: 16}, true
	default:
		return keyProfileDefinition{}, false
	}
}

func (p KeyProfile) Name() string {
	definition, _ := p.definition()
	return definition.name
}

// New generates a candidate in this Profile. Generation alone does not claim
// uniqueness; products that require uniqueness must call Allocate or implement
// the same atomic UNIQUE-constraint claim contract at their owning transaction.
func (p KeyProfile) New() (Key, error) {
	definition, ok := p.definition()
	if !ok {
		return "", ErrInvalidProfile
	}
	value, err := randomText(rand.Reader, definition.alphabet, definition.length)
	if err != nil {
		return "", err
	}
	return Key(value), nil
}

// Parse accepts only this Profile's canonical representation.
func (p KeyProfile) Parse(text string) (Key, error) {
	definition, ok := p.definition()
	if !ok {
		return "", ErrInvalidProfile
	}
	if len(text) != definition.length {
		return "", ErrInvalidKey
	}
	for index := range len(text) {
		if strings.IndexByte(definition.alphabet, text[index]) < 0 {
			return "", ErrInvalidKey
		}
	}
	return Key(text), nil
}

func randomText(reader io.Reader, alphabet string, length int) (string, error) {
	limit := 256 - (256 % len(alphabet))
	result := make([]byte, 0, length)
	buffer := make([]byte, length*2)
	if len(buffer) < 16 {
		buffer = make([]byte, 16)
	}
	for len(result) < length {
		if _, err := io.ReadFull(reader, buffer); err != nil {
			return "", fmt.Errorf("%w: %v", ErrEntropyUnavailable, err)
		}
		for _, sample := range buffer {
			if int(sample) >= limit {
				continue
			}
			result = append(result, alphabet[int(sample)%len(alphabet)])
			if len(result) == length {
				break
			}
		}
	}
	return string(result), nil
}

// ClaimResult distinguishes the one retryable allocation outcome from success.
type ClaimResult uint8

const (
	claimResultUnknown ClaimResult = iota
	Claimed
	Collision
)

// ClaimFunc must atomically insert or reserve candidate under the owning
// product's named UNIQUE constraint. Only a conflict on that exact constraint
// may return Collision.
type ClaimFunc func(context.Context, Key) (ClaimResult, error)

// Allocate generates and atomically claims a public Key with bounded collision
// retries. The claim Adapter remains product-owned so allocation shares the
// correct transaction with the row that owns the Key.
func Allocate(ctx context.Context, profile KeyProfile, claim ClaimFunc) (Key, error) {
	if _, ok := profile.definition(); !ok || claim == nil {
		return "", ErrInvalidProfile
	}
	for range MaxAllocationAttempts {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		candidate, err := profile.New()
		if err != nil {
			return "", err
		}
		result, err := claim(ctx, candidate)
		if err != nil {
			return "", err
		}
		switch result {
		case Claimed:
			return candidate, nil
		case Collision:
			continue
		default:
			return "", errors.New("identifier: claim returned an invalid result")
		}
	}
	return "", ErrCollisionExhausted
}
