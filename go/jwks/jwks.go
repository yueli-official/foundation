// Package jwks resolves public JSON Web Keys from static or remote key sets.
//
// RemoteSource is safe for concurrent use. It coalesces refreshes, bounds each
// network attempt, throttles unknown key IDs, and keeps known stale keys usable
// while an issuer is temporarily unavailable. Signature algorithm and claims
// policy deliberately belong to a verifier package, not this transport module.
package jwks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

const (
	defaultTTL          = 15 * time.Minute
	defaultMinRefresh   = time.Minute
	defaultFetchTimeout = 10 * time.Second
	defaultMaxBodyBytes = int64(1 << 20)
)

var (
	// ErrUnknownKey means the current set has no public signing key for a key ID.
	ErrUnknownKey = errors.New("jwks: unknown key")
	// ErrEmptyKeyID means a caller did not provide the required token key ID.
	ErrEmptyKeyID = errors.New("jwks: empty key ID")
	// ErrBodyTooLarge means a remote response exceeded the configured limit.
	ErrBodyTooLarge = errors.New("jwks: response body too large")
	// ErrNoUsableKeys means a set contains no valid public signature keys.
	ErrNoUsableKeys = errors.New("jwks: no usable public signature keys")
	// ErrDuplicateKeyID means a set ambiguously maps one ID to multiple keys.
	ErrDuplicateKeyID = errors.New("jwks: duplicate key ID")
	// ErrPrivateKey means a remote or static set contains private key material.
	ErrPrivateKey = errors.New("jwks: private key material is not allowed")
)

// KeySource resolves the public signing key identified by a JWT kid header.
type KeySource interface {
	PublicKey(ctx context.Context, kid string) (any, error)
}

// StaticSource resolves keys from an immutable in-memory set.
type StaticSource struct {
	keys map[string]any
}

// NewStaticSource validates and copies the usable public signature keys in set.
func NewStaticSource(set jose.JSONWebKeySet) (*StaticSource, error) {
	keys, err := indexKeys(set)
	if err != nil {
		return nil, err
	}
	return &StaticSource{keys: keys}, nil
}

// PublicKey returns the public key matching kid.
func (source *StaticSource) PublicKey(ctx context.Context, kid string) (any, error) {
	if ctx == nil {
		return nil, errors.New("jwks: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(kid) == "" {
		return nil, ErrEmptyKeyID
	}
	key, ok := source.keys[kid]
	if !ok {
		return nil, ErrUnknownKey
	}
	return key, nil
}

// RemoteOptions configures a RemoteSource. Zero values select secure defaults.
type RemoteOptions struct {
	// Client supplies transport, proxy, TLS, cookie jar and tracing behavior. A
	// shallow copy is made and redirects are always disabled.
	Client *http.Client
	// TTL controls when a successfully fetched set becomes stale.
	TTL time.Duration
	// MinRefresh limits refresh frequency, including unknown-key requests.
	MinRefresh time.Duration
	// FetchTimeout bounds the shared refresh after its initiating caller leaves.
	FetchTimeout time.Duration
	// MaxBodyBytes bounds the decoded response body.
	MaxBodyBytes int64
	// AllowLoopbackHTTP permits plain HTTP only for loopback development and tests.
	AllowLoopbackHTTP bool
}

// RemoteSource fetches and caches one issuer JWKS endpoint.
type RemoteSource struct {
	url          string
	client       *http.Client
	ttl          time.Duration
	minRefresh   time.Duration
	fetchTimeout time.Duration
	maxBodyBytes int64
	now          func() time.Time

	mu          sync.Mutex
	keys        map[string]any
	fetchedAt   time.Time
	lastAttempt time.Time
	flight      *refreshFlight
}

type refreshFlight struct {
	done chan struct{}
	err  error
}

// NewRemoteSource validates endpoint and returns a concurrency-safe source.
func NewRemoteSource(endpoint string, options RemoteOptions) (*RemoteSource, error) {
	if err := validateEndpoint(endpoint, options.AllowLoopbackHTTP); err != nil {
		return nil, err
	}
	ttl, err := positiveOrDefault("TTL", options.TTL, defaultTTL)
	if err != nil {
		return nil, err
	}
	minRefresh, err := positiveOrDefault("MinRefresh", options.MinRefresh, defaultMinRefresh)
	if err != nil {
		return nil, err
	}
	fetchTimeout, err := positiveOrDefault("FetchTimeout", options.FetchTimeout, defaultFetchTimeout)
	if err != nil {
		return nil, err
	}
	maxBodyBytes := options.MaxBodyBytes
	if maxBodyBytes == 0 {
		maxBodyBytes = defaultMaxBodyBytes
	}
	if maxBodyBytes < 0 {
		return nil, errors.New("jwks: MaxBodyBytes must be positive")
	}

	client := http.DefaultClient
	if options.Client != nil {
		client = options.Client
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &RemoteSource{
		url:          endpoint,
		client:       &clientCopy,
		ttl:          ttl,
		minRefresh:   minRefresh,
		fetchTimeout: fetchTimeout,
		maxBodyBytes: maxBodyBytes,
		now:          time.Now,
	}, nil
}

// PublicKey returns the key for kid, refreshing on expiry or a missing key ID.
// Known stale keys are served immediately while one bounded refresh continues.
func (source *RemoteSource) PublicKey(ctx context.Context, kid string) (any, error) {
	if ctx == nil {
		return nil, errors.New("jwks: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(kid) == "" {
		return nil, ErrEmptyKeyID
	}

	now := source.now()
	source.mu.Lock()
	key, known := source.keys[kid]
	fresh := source.keys != nil && now.Sub(source.fetchedAt) < source.ttl
	if known && fresh {
		source.mu.Unlock()
		return key, nil
	}

	flight := source.flight
	if flight != nil {
		source.mu.Unlock()
		if known {
			return key, nil
		}
		return source.waitForRefresh(ctx, kid, flight)
	}

	canRefresh := source.lastAttempt.IsZero() || now.Sub(source.lastAttempt) >= source.minRefresh
	if !canRefresh {
		source.mu.Unlock()
		if known {
			return key, nil
		}
		return nil, ErrUnknownKey
	}

	flight = &refreshFlight{done: make(chan struct{})}
	source.flight = flight
	source.lastAttempt = now
	refreshContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), source.fetchTimeout)
	go source.refresh(refreshContext, cancel, flight)
	source.mu.Unlock()

	if known {
		return key, nil
	}
	return source.waitForRefresh(ctx, kid, flight)
}

func (source *RemoteSource) waitForRefresh(ctx context.Context, kid string, flight *refreshFlight) (any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-flight.done:
	}

	source.mu.Lock()
	key, known := source.keys[kid]
	err := flight.err
	source.mu.Unlock()
	if known {
		return key, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, ErrUnknownKey
}

func (source *RemoteSource) refresh(ctx context.Context, cancel context.CancelFunc, flight *refreshFlight) {
	defer cancel()
	keys, err := fetch(ctx, source.client, source.url, source.maxBodyBytes)

	source.mu.Lock()
	if err == nil {
		source.keys = keys
		source.fetchedAt = source.now()
	}
	source.lastAttempt = source.now()
	flight.err = err
	close(flight.done)
	if source.flight == flight {
		source.flight = nil
	}
	source.mu.Unlock()
}

func validateEndpoint(endpoint string, allowLoopbackHTTP bool) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return errors.New("jwks: endpoint must be an absolute URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return errors.New("jwks: endpoint must not contain credentials or a fragment")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return nil
	case "http":
		if allowLoopbackHTTP && isLoopbackHost(parsed.Hostname()) {
			return nil
		}
		return errors.New("jwks: plain HTTP is allowed only for an explicitly enabled loopback endpoint")
	default:
		return errors.New("jwks: endpoint scheme must be HTTPS")
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func positiveOrDefault(name string, value, fallback time.Duration) (time.Duration, error) {
	if value == 0 {
		return fallback, nil
	}
	if value < 0 {
		return 0, fmt.Errorf("jwks: %s must be positive", name)
	}
	return value, nil
}

func indexKeys(set jose.JSONWebKeySet) (map[string]any, error) {
	keys := make(map[string]any, len(set.Keys))
	for index := range set.Keys {
		key := &set.Keys[index]
		if !key.Valid() {
			continue
		}
		if !key.IsPublic() {
			return nil, ErrPrivateKey
		}
		if key.Use != "" && key.Use != "sig" {
			continue
		}
		if strings.TrimSpace(key.KeyID) == "" {
			continue
		}
		if _, exists := keys[key.KeyID]; exists {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateKeyID, key.KeyID)
		}
		keys[key.KeyID] = key.Key
	}
	if len(keys) == 0 {
		return nil, ErrNoUsableKeys
	}
	return keys, nil
}
