package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	defaultLeeway       = 30 * time.Second
	defaultMaxTokenSize = 16 << 10
)

// KeySource resolves a public verification key. Package jwks sources satisfy
// this Interface without auth depending on a particular transport package.
type KeySource interface {
	PublicKey(ctx context.Context, kid string) (any, error)
}

// Config defines access-token verification policy. Issuer and Keys are
// required. Empty Algorithms selects RS256. Configured Audiences use any-match
// semantics. Empty Types disables typ checking; set []string{"at+jwt"} when the
// issuer follows RFC 9068 to prevent cross-JWT confusion.
type Config struct {
	Keys       KeySource
	Issuer     string
	Audiences  []string
	Algorithms []jose.SignatureAlgorithm
	Types      []string
	// Leeway defaults to 30 seconds when zero and must not be negative.
	Leeway time.Duration
	// MaxTokenBytes defaults to 16 KiB when zero.
	MaxTokenBytes int
	// MaxLifetime, when positive, requires iat and limits exp-iat.
	MaxLifetime time.Duration
	// AllowActorless permits a token with neither sub nor client_id.
	AllowActorless bool
}

// Verifier is immutable and safe for concurrent use.
type Verifier struct {
	keys           KeySource
	issuer         string
	audiences      jwt.Audience
	algorithms     []jose.SignatureAlgorithm
	types          []string
	leeway         time.Duration
	maxTokenBytes  int
	maxLifetime    time.Duration
	allowActorless bool
	now            func() time.Time
}

// NewVerifier validates and copies policy from config.
func NewVerifier(config Config) (*Verifier, error) {
	if config.Keys == nil {
		return nil, errors.New("auth: Keys is required")
	}
	if strings.TrimSpace(config.Issuer) == "" {
		return nil, errors.New("auth: Issuer is required")
	}
	algorithms := config.Algorithms
	if len(algorithms) == 0 {
		algorithms = []jose.SignatureAlgorithm{jose.RS256}
	}
	algorithms, err := validateAlgorithms(algorithms)
	if err != nil {
		return nil, err
	}
	audiences, err := validateStrings("Audiences", config.Audiences)
	if err != nil {
		return nil, err
	}
	types, err := validateStrings("Types", config.Types)
	if err != nil {
		return nil, err
	}
	if config.Leeway < 0 {
		return nil, errors.New("auth: Leeway must not be negative")
	}
	leeway := config.Leeway
	if leeway == 0 {
		leeway = defaultLeeway
	}
	maxTokenBytes := config.MaxTokenBytes
	if maxTokenBytes == 0 {
		maxTokenBytes = defaultMaxTokenSize
	}
	if maxTokenBytes < 0 {
		return nil, errors.New("auth: MaxTokenBytes must be positive")
	}
	if config.MaxLifetime < 0 {
		return nil, errors.New("auth: MaxLifetime must not be negative")
	}

	return &Verifier{
		keys:           config.Keys,
		issuer:         config.Issuer,
		audiences:      jwt.Audience(audiences),
		algorithms:     algorithms,
		types:          types,
		leeway:         leeway,
		maxTokenBytes:  maxTokenBytes,
		maxLifetime:    config.MaxLifetime,
		allowActorless: config.AllowActorless,
		now:            time.Now,
	}, nil
}

// Verify authenticates a compact signed JWT and returns its typed Principal.
func (verifier *Verifier) Verify(ctx context.Context, raw string) (*Principal, error) {
	if ctx == nil {
		return nil, errors.New("auth: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(raw) > verifier.maxTokenBytes {
		return nil, ErrTokenTooLarge
	}
	algorithm, err := compactAlgorithm(raw)
	if err != nil {
		return nil, err
	}
	if !slicesContains(verifier.algorithms, jose.SignatureAlgorithm(algorithm)) {
		return nil, ErrUnsupportedAlgorithm
	}

	token, err := jwt.ParseSigned(raw, verifier.algorithms)
	if err != nil || len(token.Headers) != 1 {
		return nil, ErrMalformedToken
	}
	header := token.Headers[0]
	if !typeAllowed(header, verifier.types) {
		return nil, ErrInvalidType
	}
	if strings.TrimSpace(header.KeyID) == "" {
		return nil, ErrMissingKeyID
	}
	key, err := verifier.keys.PublicKey(ctx, header.KeyID)
	if err != nil {
		if contextError := ctx.Err(); contextError != nil {
			return nil, contextError
		}
		return nil, fmt.Errorf("%w: %w", ErrKeyUnavailable, err)
	}
	if key == nil {
		return nil, ErrKeyUnavailable
	}

	var payload json.RawMessage
	if err := token.Claims(key, &payload); err != nil {
		return nil, ErrBadSignature
	}
	var standard jwt.Claims
	var extension struct {
		Scope    string   `json:"scope"`
		Roles    []string `json:"roles"`
		ClientID string   `json:"client_id"`
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &standard); err != nil {
		return nil, ErrInvalidClaims
	}
	if err := json.Unmarshal(payload, &extension); err != nil {
		return nil, ErrInvalidClaims
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims == nil {
		return nil, ErrInvalidClaims
	}
	if standard.Expiry == nil {
		return nil, ErrMissingExpiry
	}
	expected := jwt.Expected{
		Issuer:      verifier.issuer,
		AnyAudience: verifier.audiences,
		Time:        verifier.now(),
	}
	if err := standard.ValidateWithLeeway(expected, verifier.leeway); err != nil {
		return nil, mapValidationError(err)
	}
	if verifier.maxLifetime > 0 {
		if standard.IssuedAt == nil || !standard.Expiry.Time().After(standard.IssuedAt.Time()) || standard.Expiry.Time().Sub(standard.IssuedAt.Time()) > verifier.maxLifetime {
			return nil, ErrInvalidLifetime
		}
	}
	if !verifier.allowActorless && strings.TrimSpace(standard.Subject) == "" && strings.TrimSpace(extension.ClientID) == "" {
		return nil, ErrMissingActor
	}

	return &Principal{
		Subject:   standard.Subject,
		ClientID:  extension.ClientID,
		Issuer:    standard.Issuer,
		Audience:  append([]string{}, standard.Audience...),
		Scopes:    uniqueStrings(strings.Fields(extension.Scope)),
		Roles:     uniqueStrings(extension.Roles),
		IssuedAt:  numericDateTime(standard.IssuedAt),
		ExpiresAt: standard.Expiry.Time(),
		claims:    claims,
	}, nil
}

func compactAlgorithm(raw string) (string, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", ErrMalformedToken
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", ErrMalformedToken
	}
	var header struct {
		Algorithm string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil || header.Algorithm == "" {
		return "", ErrMalformedToken
	}
	return header.Algorithm, nil
}

func typeAllowed(header jose.Header, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	value, ok := header.ExtraHeaders[jose.HeaderType].(string)
	if !ok {
		return false
	}
	for _, candidate := range allowed {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

func validateAlgorithms(values []jose.SignatureAlgorithm) ([]jose.SignatureAlgorithm, error) {
	result := make([]jose.SignatureAlgorithm, 0, len(values))
	for _, value := range values {
		if !asymmetricAlgorithm(value) {
			return nil, fmt.Errorf("auth: signing algorithm %q is not an approved asymmetric algorithm", value)
		}
		if !slicesContains(result, value) {
			result = append(result, value)
		}
	}
	return result, nil
}

func asymmetricAlgorithm(value jose.SignatureAlgorithm) bool {
	switch value {
	case jose.RS256, jose.RS384, jose.RS512,
		jose.PS256, jose.PS384, jose.PS512,
		jose.ES256, jose.ES384, jose.ES512,
		jose.EdDSA:
		return true
	default:
		return false
	}
}

func validateStrings(name string, values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("auth: %s must not contain empty values", name)
		}
		if !slicesContains(result, value) {
			result = append(result, value)
		}
	}
	return result, nil
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !slicesContains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func slicesContains[T comparable](values []T, target T) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func numericDateTime(value *jwt.NumericDate) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.Time()
}

func mapValidationError(err error) error {
	switch {
	case errors.Is(err, jwt.ErrExpired):
		return ErrExpired
	case errors.Is(err, jwt.ErrNotValidYet), errors.Is(err, jwt.ErrIssuedInTheFuture):
		return ErrNotYetValid
	case errors.Is(err, jwt.ErrInvalidIssuer):
		return ErrInvalidIssuer
	case errors.Is(err, jwt.ErrInvalidAudience):
		return ErrInvalidAudience
	default:
		return ErrInvalidClaims
	}
}
