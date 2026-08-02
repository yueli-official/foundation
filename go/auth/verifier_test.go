package auth_test

import (
	"context"
	"crypto/ed25519"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/yueli-official/foundation/go/auth"
	"github.com/yueli-official/foundation/go/jwks"
)

const (
	testIssuer = "https://identity.example.test"
	testKeyID  = "key-1"
)

type keySourceFunc func(context.Context, string) (any, error)

func (function keySourceFunc) PublicKey(ctx context.Context, kid string) (any, error) {
	return function(ctx, kid)
}

func TestVerifierReturnsTypedPrincipal(t *testing.T) {
	private := testPrivateKey(1)
	verifier := newVerifier(t, private.Public(), auth.Config{
		Audiences:  []string{"resource-api"},
		Algorithms: []jose.SignatureAlgorithm{jose.EdDSA},
		Types:      []string{"at+jwt"},
	})
	now := time.Now().UTC().Truncate(time.Second)
	raw := sign(t, private, jose.EdDSA, testKeyID, "at+jwt", jwt.Claims{
		Issuer:   testIssuer,
		Subject:  "user-1",
		Audience: jwt.Audience{"other", "resource-api"},
		IssuedAt: jwt.NewNumericDate(now),
		Expiry:   jwt.NewNumericDate(now.Add(5 * time.Minute)),
	}, map[string]any{
		"scope":        "openid profile profile",
		"roles":        []string{"editor", "editor"},
		"client_id":    "web-client",
		"subject_kind": "user",
		"tenant_id":    "tenant-1",
	})

	principal, err := verifier.Verify(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if principal.ActorKey() != "user-1" || principal.ClientID != "web-client" {
		t.Fatalf("principal actor = %q, client = %q", principal.ActorKey(), principal.ClientID)
	}
	if !principal.IsUser() || principal.SubjectKind != auth.SubjectUser {
		t.Fatalf("principal subject kind = %q", principal.SubjectKind)
	}
	if !principal.HasScope("profile") || len(principal.Scopes) != 2 {
		t.Fatalf("scopes = %#v", principal.Scopes)
	}
	if !principal.HasRole("editor") || len(principal.Roles) != 1 {
		t.Fatalf("roles = %#v", principal.Roles)
	}
	if tenant, ok := principal.Claim("tenant_id"); !ok || tenant != "tenant-1" {
		t.Fatalf("tenant claim = %#v, %v", tenant, ok)
	}
}

func TestJWKSStaticSourceSatisfiesAuthKeySource(t *testing.T) {
	private := testPrivateKey(10)
	source, err := jwks.NewStaticSource(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: private.Public(), KeyID: testKeyID, Use: "sig", Algorithm: "EdDSA",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := auth.NewVerifier(auth.Config{
		Keys: source, Issuer: testIssuer, Algorithms: []jose.SignatureAlgorithm{jose.EdDSA},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	raw := sign(t, private, jose.EdDSA, testKeyID, "", jwt.Claims{
		Issuer: testIssuer, Subject: "user", Expiry: jwt.NewNumericDate(now.Add(time.Minute)),
	}, nil)
	if _, err := verifier.Verify(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
}

func TestVerifierRejectsInvalidConfiguration(t *testing.T) {
	validKeys := keySourceFunc(func(context.Context, string) (any, error) { return testPrivateKey(2).Public(), nil })
	tests := []struct {
		name   string
		config auth.Config
	}{
		{name: "missing keys", config: auth.Config{Issuer: testIssuer}},
		{name: "missing issuer", config: auth.Config{Keys: validKeys}},
		{name: "symmetric algorithm", config: auth.Config{Keys: validKeys, Issuer: testIssuer, Algorithms: []jose.SignatureAlgorithm{jose.HS256}}},
		{name: "empty audience", config: auth.Config{Keys: validKeys, Issuer: testIssuer, Audiences: []string{""}}},
		{name: "negative leeway", config: auth.Config{Keys: validKeys, Issuer: testIssuer, Leeway: -time.Second}},
		{name: "negative token size", config: auth.Config{Keys: validKeys, Issuer: testIssuer, MaxTokenBytes: -1}},
		{name: "negative lifetime", config: auth.Config{Keys: validKeys, Issuer: testIssuer, MaxLifetime: -time.Second}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := auth.NewVerifier(test.config); err == nil {
				t.Fatal("NewVerifier() accepted invalid config")
			}
		})
	}
}

func TestVerifierRequiresExpiryAndActorByDefault(t *testing.T) {
	private := testPrivateKey(3)
	verifier := newVerifier(t, private.Public(), auth.Config{Algorithms: []jose.SignatureAlgorithm{jose.EdDSA}})
	now := time.Now().UTC().Truncate(time.Second)
	missingExpiry := sign(t, private, jose.EdDSA, testKeyID, "", jwt.Claims{
		Issuer:  testIssuer,
		Subject: "user-1",
	}, nil)
	if _, err := verifier.Verify(context.Background(), missingExpiry); !errors.Is(err, auth.ErrMissingExpiry) {
		t.Fatalf("missing expiry error = %v", err)
	}
	missingActor := sign(t, private, jose.EdDSA, testKeyID, "", jwt.Claims{
		Issuer: testIssuer,
		Expiry: jwt.NewNumericDate(now.Add(time.Minute)),
	}, nil)
	if _, err := verifier.Verify(context.Background(), missingActor); !errors.Is(err, auth.ErrMissingActor) {
		t.Fatalf("missing actor error = %v", err)
	}
}

func TestVerifierRequiresSubjectKindForActors(t *testing.T) {
	private := testPrivateKey(12)
	verifier := newVerifier(t, private.Public(), auth.Config{Algorithms: []jose.SignatureAlgorithm{jose.EdDSA}})
	now := time.Now().UTC().Truncate(time.Second)
	claims := jwt.Claims{
		Issuer: testIssuer, Subject: "user-1", Expiry: jwt.NewNumericDate(now.Add(time.Minute)),
	}

	tests := []struct {
		name  string
		extra map[string]any
	}{
		{name: "missing kind", extra: map[string]any{"subject_kind": nil}},
		{name: "unknown kind", extra: map[string]any{"subject_kind": "robot"}},
		{name: "client kind without client ID", extra: map[string]any{"subject_kind": "client"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := sign(t, private, jose.EdDSA, testKeyID, "", claims, test.extra)
			if _, err := verifier.Verify(context.Background(), raw); !errors.Is(err, auth.ErrInvalidSubjectKind) {
				t.Fatalf("Verify() error = %v, want %v", err, auth.ErrInvalidSubjectKind)
			}
		})
	}
}

func TestClientActorKeyDoesNotUseTokenSubject(t *testing.T) {
	private := testPrivateKey(13)
	verifier := newVerifier(t, private.Public(), auth.Config{Algorithms: []jose.SignatureAlgorithm{jose.EdDSA}})
	now := time.Now().UTC().Truncate(time.Second)
	raw := sign(t, private, jose.EdDSA, testKeyID, "", jwt.Claims{
		Issuer: testIssuer, Subject: "client-subject-that-is-not-a-user", Expiry: jwt.NewNumericDate(now.Add(time.Minute)),
	}, map[string]any{"subject_kind": "client", "client_id": "worker-client"})

	principal, err := verifier.Verify(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if principal.ActorKey() != "worker-client" || principal.IsUser() {
		t.Fatalf("client actor = %q, isUser = %v", principal.ActorKey(), principal.IsUser())
	}
}

func TestVerifierValidatesTimeIssuerAndAnyAudience(t *testing.T) {
	private := testPrivateKey(4)
	verifier := newVerifier(t, private.Public(), auth.Config{
		Algorithms: []jose.SignatureAlgorithm{jose.EdDSA},
		Audiences:  []string{"api-a", "api-b"},
	})
	now := time.Now().UTC().Truncate(time.Second)
	tests := []struct {
		name   string
		claims jwt.Claims
		want   error
	}{
		{name: "expired", claims: jwt.Claims{Issuer: testIssuer, Subject: "user", Audience: jwt.Audience{"api-a"}, Expiry: jwt.NewNumericDate(now.Add(-time.Minute))}, want: auth.ErrExpired},
		{name: "not yet valid", claims: jwt.Claims{Issuer: testIssuer, Subject: "user", Audience: jwt.Audience{"api-a"}, NotBefore: jwt.NewNumericDate(now.Add(time.Minute)), Expiry: jwt.NewNumericDate(now.Add(2 * time.Minute))}, want: auth.ErrNotYetValid},
		{name: "wrong issuer", claims: jwt.Claims{Issuer: "https://wrong.example.test", Subject: "user", Audience: jwt.Audience{"api-a"}, Expiry: jwt.NewNumericDate(now.Add(time.Minute))}, want: auth.ErrInvalidIssuer},
		{name: "wrong audience", claims: jwt.Claims{Issuer: testIssuer, Subject: "user", Audience: jwt.Audience{"other"}, Expiry: jwt.NewNumericDate(now.Add(time.Minute))}, want: auth.ErrInvalidAudience},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := sign(t, private, jose.EdDSA, testKeyID, "", test.claims, nil)
			if _, err := verifier.Verify(context.Background(), raw); !errors.Is(err, test.want) {
				t.Fatalf("Verify() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestVerifierEnforcesAlgorithmBeforeResolvingKey(t *testing.T) {
	private := testPrivateKey(5)
	var calls atomic.Int32
	keys := keySourceFunc(func(context.Context, string) (any, error) {
		calls.Add(1)
		return private.Public(), nil
	})
	verifier, err := auth.NewVerifier(auth.Config{Keys: keys, Issuer: testIssuer})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	raw := sign(t, private, jose.EdDSA, testKeyID, "", jwt.Claims{
		Issuer: testIssuer, Subject: "user", Expiry: jwt.NewNumericDate(now.Add(time.Minute)),
	}, nil)
	if _, err := verifier.Verify(context.Background(), raw); !errors.Is(err, auth.ErrUnsupportedAlgorithm) {
		t.Fatalf("algorithm error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("key source calls = %d, want 0", calls.Load())
	}
}

func TestVerifierRejectsBadSignatureTypeAndMissingKeyID(t *testing.T) {
	private := testPrivateKey(6)
	other := testPrivateKey(7)
	verifier := newVerifier(t, private.Public(), auth.Config{
		Algorithms: []jose.SignatureAlgorithm{jose.EdDSA}, Types: []string{"at+jwt"},
	})
	now := time.Now().UTC()
	claims := jwt.Claims{Issuer: testIssuer, Subject: "user", Expiry: jwt.NewNumericDate(now.Add(time.Minute))}

	badSignature := sign(t, other, jose.EdDSA, testKeyID, "at+jwt", claims, nil)
	if _, err := verifier.Verify(context.Background(), badSignature); !errors.Is(err, auth.ErrBadSignature) {
		t.Fatalf("signature error = %v", err)
	}
	wrongType := sign(t, private, jose.EdDSA, testKeyID, "JWT", claims, nil)
	if _, err := verifier.Verify(context.Background(), wrongType); !errors.Is(err, auth.ErrInvalidType) {
		t.Fatalf("type error = %v", err)
	}
	missingKeyID := sign(t, private, jose.EdDSA, "", "at+jwt", claims, nil)
	if _, err := verifier.Verify(context.Background(), missingKeyID); !errors.Is(err, auth.ErrMissingKeyID) {
		t.Fatalf("key ID error = %v", err)
	}
}

func TestVerifierBoundsTokenLifetimeAndSize(t *testing.T) {
	private := testPrivateKey(8)
	verifier := newVerifier(t, private.Public(), auth.Config{
		Algorithms:    []jose.SignatureAlgorithm{jose.EdDSA},
		MaxLifetime:   5 * time.Minute,
		MaxTokenBytes: 4096,
	})
	now := time.Now().UTC().Truncate(time.Second)
	tooLong := sign(t, private, jose.EdDSA, testKeyID, "", jwt.Claims{
		Issuer: testIssuer, Subject: "user", IssuedAt: jwt.NewNumericDate(now), Expiry: jwt.NewNumericDate(now.Add(10 * time.Minute)),
	}, nil)
	if _, err := verifier.Verify(context.Background(), tooLong); !errors.Is(err, auth.ErrInvalidLifetime) {
		t.Fatalf("lifetime error = %v", err)
	}
	if _, err := verifier.Verify(context.Background(), strings.Repeat("x", 4097)); !errors.Is(err, auth.ErrTokenTooLarge) {
		t.Fatalf("size error = %v", err)
	}
}

func TestVerifierUsesConfiguredClock(t *testing.T) {
	private := testPrivateKey(11)
	now := time.Date(2026, 7, 22, 6, 0, 0, 0, time.UTC)
	verifier := newVerifier(t, private.Public(), auth.Config{
		Algorithms: []jose.SignatureAlgorithm{jose.EdDSA},
		Clock:      func() time.Time { return now.Add(2 * time.Minute) },
	})
	raw := sign(t, private, jose.EdDSA, testKeyID, "", jwt.Claims{
		Issuer: testIssuer, Subject: "user", Expiry: jwt.NewNumericDate(now.Add(time.Minute)),
	}, nil)
	if _, err := verifier.Verify(context.Background(), raw); !errors.Is(err, auth.ErrExpired) {
		t.Fatalf("configured clock error = %v", err)
	}
}

func TestVerifierPreservesContextAndWrapsKeyFailure(t *testing.T) {
	private := testPrivateKey(9)
	now := time.Now().UTC()
	raw := sign(t, private, jose.EdDSA, testKeyID, "", jwt.Claims{
		Issuer: testIssuer, Subject: "user", Expiry: jwt.NewNumericDate(now.Add(time.Minute)),
	}, nil)
	keyFailure := errors.New("issuer unavailable")
	verifier, err := auth.NewVerifier(auth.Config{
		Keys: keySourceFunc(func(context.Context, string) (any, error) {
			return nil, keyFailure
		}),
		Issuer: testIssuer, Algorithms: []jose.SignatureAlgorithm{jose.EdDSA},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(context.Background(), raw); !errors.Is(err, auth.ErrKeyUnavailable) || !errors.Is(err, keyFailure) {
		t.Fatalf("key failure = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := verifier.Verify(ctx, raw); !errors.Is(err, context.Canceled) {
		t.Fatalf("context error = %v", err)
	}
}

func newVerifier(t *testing.T, public any, config auth.Config) *auth.Verifier {
	t.Helper()
	config.Keys = keySourceFunc(func(_ context.Context, kid string) (any, error) {
		if kid != testKeyID {
			return nil, errors.New("unknown key")
		}
		return public, nil
	})
	config.Issuer = testIssuer
	verifier, err := auth.NewVerifier(config)
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}

func sign(t *testing.T, private ed25519.PrivateKey, algorithm jose.SignatureAlgorithm, keyID, tokenType string, claims jwt.Claims, extra map[string]any) string {
	t.Helper()
	options := (&jose.SignerOptions{}).WithHeader("kid", keyID)
	if tokenType != "" {
		options.WithType(jose.ContentType(tokenType))
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: algorithm, Key: private}, options)
	if err != nil {
		t.Fatal(err)
	}
	if extra == nil {
		extra = map[string]any{}
	}
	if _, declared := extra["subject_kind"]; !declared {
		if strings.TrimSpace(claims.Subject) != "" {
			extra["subject_kind"] = "user"
		} else if clientID, _ := extra["client_id"].(string); strings.TrimSpace(clientID) != "" {
			extra["subject_kind"] = "client"
		}
	}
	builder := jwt.Signed(signer).Claims(claims)
	if extra != nil {
		builder = builder.Claims(extra)
	}
	raw, err := builder.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func testPrivateKey(seed byte) ed25519.PrivateKey {
	bytes := make([]byte, ed25519.SeedSize)
	for index := range bytes {
		bytes[index] = seed
	}
	return ed25519.NewKeyFromSeed(bytes)
}
