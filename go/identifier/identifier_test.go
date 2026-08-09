package identifier_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/yueli-official/foundation/go/identifier"
)

func TestNewCreatesCanonicalUUIDv7(t *testing.T) {
	first, err := identifier.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	second := identifier.MustNew()
	if first == second {
		t.Fatal("independent UUIDs repeated")
	}
	for _, value := range []identifier.UUID{first, second} {
		if value.Version() != 7 || value.Variant() != uuid.RFC4122 {
			t.Fatalf("UUID %q has version %d variant %v, want RFC UUIDv7", value, value.Version(), value.Variant())
		}
		if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(value.String()) {
			t.Fatalf("UUID %q is not canonical lowercase UUIDv7", value)
		}
	}
}

func TestParseRequiresCanonicalUUIDText(t *testing.T) {
	value := identifier.MustNew()
	parsed, err := identifier.Parse(value.String())
	if err != nil {
		t.Fatalf("Parse canonical UUID: %v", err)
	}
	if parsed != value {
		t.Fatalf("Parse = %v, want %v", parsed, value)
	}
	for _, text := range []string{
		"", " " + value.String(), value.String() + " ",
		"{" + value.String() + "}",
		"urn:uuid:" + value.String(),
		value.String()[:8] + value.String()[9:13] + value.String()[14:18] + value.String()[19:23] + value.String()[24:],
	} {
		if _, err := identifier.Parse(text); !errors.Is(err, identifier.ErrInvalidUUID) {
			t.Errorf("Parse(%q) error = %v, want ErrInvalidUUID", text, err)
		}
	}
}

func TestDeriveIsStableUUIDv5(t *testing.T) {
	namespace := uuid.MustParse("2f1fb888-699f-48f6-80f0-8c37d3fa6af7")
	first := identifier.Derive(namespace, []byte("identity.user|v1|external:42"))
	second := identifier.Derive(namespace, []byte("identity.user|v1|external:42"))
	other := identifier.Derive(namespace, []byte("identity.user|v1|external:43"))
	if first != second || first == other {
		t.Fatalf("deterministic derivation mismatch: first=%v second=%v other=%v", first, second, other)
	}
	if first.Version() != 5 || first.Variant() != uuid.RFC4122 {
		t.Fatalf("derived UUID version=%d variant=%v, want RFC UUIDv5", first.Version(), first.Variant())
	}
	if got, want := first.String(), "28a84079-8ff7-5e51-93ad-2e54d6255597"; got != want {
		t.Fatalf("derived UUID = %s, want cross-language vector %s", got, want)
	}
}

func TestPublishedKeyProfilesGenerateAndParseTheirCanonicalShape(t *testing.T) {
	tests := []struct {
		name    string
		profile identifier.KeyProfile
		pattern string
		length  int
	}{
		{name: "compact URL", profile: identifier.CompactURLV1, pattern: `^[1-9A-HJ-NP-Za-km-z]{8}$`, length: 8},
		{name: "human code", profile: identifier.HumanCodeV1, pattern: `^[0-9A-HJKMNP-TV-Z]{10}$`, length: 10},
		{name: "opaque public", profile: identifier.OpaquePublicV1, pattern: `^[0-9A-HJKMNP-TV-Z]{16}$`, length: 16},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			seen := map[identifier.Key]bool{}
			for range 128 {
				key, err := test.profile.New()
				if err != nil {
					t.Fatalf("New: %v", err)
				}
				if len(key.String()) != test.length || !regexp.MustCompile(test.pattern).MatchString(key.String()) {
					t.Fatalf("generated key %q does not match %s", key, test.pattern)
				}
				parsed, err := test.profile.Parse(key.String())
				if err != nil || parsed != key {
					t.Fatalf("Parse(%q) = %q, %v", key, parsed, err)
				}
				if seen[key] {
					t.Fatalf("key repeated in small sample: %q", key)
				}
				seen[key] = true
			}
		})
	}
}

func TestKeyProfileRejectsWrongOrNonCanonicalText(t *testing.T) {
	for _, test := range []struct {
		profile identifier.KeyProfile
		values  []string
	}{
		{identifier.CompactURLV1, []string{"", "1234567", "123456789", "0ABCDEFG", "OABCDEFG"}},
		{identifier.HumanCodeV1, []string{"", "ABCD2345", "ABCD2345678", "abcd234567", "ABCI234567"}},
		{identifier.OpaquePublicV1, []string{"", "ABCD2345", "abcd23456789abcd", "ABCO23456789ABCD"}},
	} {
		for _, value := range test.values {
			if _, err := test.profile.Parse(value); !errors.Is(err, identifier.ErrInvalidKey) {
				t.Errorf("%s.Parse(%q) error = %v, want ErrInvalidKey", test.profile.Name(), value, err)
			}
		}
	}
}

func TestAllocateRetriesOnlyDeclaredCollisions(t *testing.T) {
	attempts := 0
	allocated, err := identifier.Allocate(context.Background(), identifier.CompactURLV1,
		func(_ context.Context, candidate identifier.Key) (identifier.ClaimResult, error) {
			attempts++
			if attempts < 3 {
				return identifier.Collision, nil
			}
			return identifier.Claimed, nil
		})
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if attempts != 3 || allocated == "" {
		t.Fatalf("attempts=%d allocated=%q, want 3 and non-empty", attempts, allocated)
	}

	want := errors.New("database unavailable")
	attempts = 0
	_, err = identifier.Allocate(context.Background(), identifier.CompactURLV1,
		func(context.Context, identifier.Key) (identifier.ClaimResult, error) {
			attempts++
			return 0, want
		})
	if !errors.Is(err, want) || attempts != 1 {
		t.Fatalf("ordinary error = %v after %d attempts, want immediate %v", err, attempts, want)
	}
}

func TestAllocateStopsAfterBoundedCollisionsAndHonorsCancellation(t *testing.T) {
	attempts := 0
	_, err := identifier.Allocate(context.Background(), identifier.CompactURLV1,
		func(context.Context, identifier.Key) (identifier.ClaimResult, error) {
			attempts++
			return identifier.Collision, nil
		})
	if !errors.Is(err, identifier.ErrCollisionExhausted) || attempts != identifier.MaxAllocationAttempts {
		t.Fatalf("collision exhaustion error=%v attempts=%d", err, attempts)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	attempts = 0
	_, err = identifier.Allocate(ctx, identifier.CompactURLV1,
		func(context.Context, identifier.Key) (identifier.ClaimResult, error) {
			attempts++
			return identifier.Claimed, nil
		})
	if !errors.Is(err, context.Canceled) || attempts != 0 {
		t.Fatalf("cancelled Allocate error=%v attempts=%d", err, attempts)
	}
}
