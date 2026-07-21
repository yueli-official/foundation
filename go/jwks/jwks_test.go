package jwks_test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/yueli-official/foundation/go/jwks"
)

func TestStaticSourceValidatesAndResolvesPublicKeys(t *testing.T) {
	key := testPublicKey("key-1", 1)
	source, err := jwks.NewStaticSource(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{key}})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := source.PublicKey(context.Background(), "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resolved, key.Key) {
		t.Fatalf("resolved key does not match: %#v", resolved)
	}
	if _, err := source.PublicKey(context.Background(), "missing"); !errors.Is(err, jwks.ErrUnknownKey) {
		t.Fatalf("missing key error = %v", err)
	}
	if _, err := source.PublicKey(context.Background(), " "); !errors.Is(err, jwks.ErrEmptyKeyID) {
		t.Fatalf("empty key error = %v", err)
	}
}

func TestStaticSourceRejectsPrivateAndDuplicateKeys(t *testing.T) {
	private := testPrivateKey("private", 2)
	if _, err := jwks.NewStaticSource(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{private}}); !errors.Is(err, jwks.ErrPrivateKey) {
		t.Fatalf("private key error = %v", err)
	}

	duplicate := testPublicKey("duplicate", 3)
	if _, err := jwks.NewStaticSource(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{duplicate, duplicate}}); !errors.Is(err, jwks.ErrDuplicateKeyID) {
		t.Fatalf("duplicate key error = %v", err)
	}
}

func TestRemoteSourceValidatesEndpointAndOptions(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		options  jwks.RemoteOptions
	}{
		{name: "relative", endpoint: "/keys"},
		{name: "credentials", endpoint: "https://user:pass@example.test/keys"},
		{name: "fragment", endpoint: "https://example.test/keys#fragment"},
		{name: "plain remote HTTP", endpoint: "http://example.test/keys", options: jwks.RemoteOptions{AllowLoopbackHTTP: true}},
		{name: "plain loopback HTTP not enabled", endpoint: "http://127.0.0.1/keys"},
		{name: "negative TTL", endpoint: "https://example.test/keys", options: jwks.RemoteOptions{TTL: -time.Second}},
		{name: "negative body limit", endpoint: "https://example.test/keys", options: jwks.RemoteOptions{MaxBodyBytes: -1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := jwks.NewRemoteSource(test.endpoint, test.options); err == nil {
				t.Fatal("NewRemoteSource() accepted invalid input")
			}
		})
	}
	if _, err := jwks.NewRemoteSource("http://localhost/keys", jwks.RemoteOptions{AllowLoopbackHTTP: true}); err != nil {
		t.Fatalf("explicit loopback HTTP rejected: %v", err)
	}
}

func TestRemoteSourceCoalescesConcurrentInitialRefresh(t *testing.T) {
	key := testPublicKey("rotating", 4)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		time.Sleep(40 * time.Millisecond)
		writeSet(t, writer, key)
	}))
	defer server.Close()
	source := newTestRemote(t, server.URL, jwks.RemoteOptions{})

	const callers = 48
	start := make(chan struct{})
	errorsFound := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			resolved, err := source.PublicKey(context.Background(), "rotating")
			if err == nil && !reflect.DeepEqual(resolved, key.Key) {
				err = errors.New("resolved key did not match")
			}
			errorsFound <- err
		}()
	}
	close(start)
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

func TestRemoteSourceThrottlesConcurrentUnknownKeyRefresh(t *testing.T) {
	key := testPublicKey("known", 5)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		time.Sleep(20 * time.Millisecond)
		writeSet(t, writer, key)
	}))
	defer server.Close()
	source := newTestRemote(t, server.URL, jwks.RemoteOptions{MinRefresh: 5 * time.Millisecond})
	if _, err := source.PublicKey(context.Background(), "known"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	const callers = 32
	start := make(chan struct{})
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			if _, err := source.PublicKey(context.Background(), "unknown"); !errors.Is(err, jwks.ErrUnknownKey) {
				t.Errorf("unknown key error = %v", err)
			}
		}()
	}
	close(start)
	group.Wait()
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests after concurrent miss = %d, want 2", got)
	}
	if _, err := source.PublicKey(context.Background(), "another-unknown"); !errors.Is(err, jwks.ErrUnknownKey) {
		t.Fatalf("throttled unknown key error = %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests after throttled miss = %d, want 2", got)
	}
}

func TestRemoteSourceRefreshesOnKeyRotation(t *testing.T) {
	oldKey := testPublicKey("old", 9)
	newKey := testPublicKey("new", 10)
	var rotated atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if rotated.Load() {
			writeSet(t, writer, newKey)
			return
		}
		writeSet(t, writer, oldKey)
	}))
	defer server.Close()
	source := newTestRemote(t, server.URL, jwks.RemoteOptions{MinRefresh: 5 * time.Millisecond})
	if _, err := source.PublicKey(context.Background(), "old"); err != nil {
		t.Fatal(err)
	}
	rotated.Store(true)
	time.Sleep(10 * time.Millisecond)
	resolved, err := source.PublicKey(context.Background(), "new")
	if err != nil || !reflect.DeepEqual(resolved, newKey.Key) {
		t.Fatalf("rotated lookup = %#v, %v", resolved, err)
	}
	if _, err := source.PublicKey(context.Background(), "old"); !errors.Is(err, jwks.ErrUnknownKey) {
		t.Fatalf("retired key error = %v", err)
	}
}

func TestRemoteSourceServesStaleKnownKeyDuringFailedRefresh(t *testing.T) {
	key := testPublicKey("known", 6)
	var fail atomic.Bool
	failedRequest := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			writer.WriteHeader(http.StatusServiceUnavailable)
			failedRequest <- struct{}{}
			return
		}
		writeSet(t, writer, key)
	}))
	defer server.Close()
	source := newTestRemote(t, server.URL, jwks.RemoteOptions{
		TTL:        5 * time.Millisecond,
		MinRefresh: time.Millisecond,
	})
	if _, err := source.PublicKey(context.Background(), "known"); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	time.Sleep(10 * time.Millisecond)

	resolved, err := source.PublicKey(context.Background(), "known")
	if err != nil || !reflect.DeepEqual(resolved, key.Key) {
		t.Fatalf("stale lookup = %#v, %v", resolved, err)
	}
	select {
	case <-failedRequest:
	case <-time.After(time.Second):
		t.Fatal("background stale refresh did not run")
	}
	resolved, err = source.PublicKey(context.Background(), "known")
	if err != nil || !reflect.DeepEqual(resolved, key.Key) {
		t.Fatalf("stale fallback = %#v, %v", resolved, err)
	}
}

func TestRemoteSourceBoundsBodyAndDisablesRedirects(t *testing.T) {
	var redirected atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/large", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(strings.Repeat("x", 65)))
	})
	mux.HandleFunc("/redirect", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/keys", http.StatusFound)
	})
	mux.HandleFunc("/keys", func(writer http.ResponseWriter, _ *http.Request) {
		redirected.Add(1)
		writeSet(t, writer, testPublicKey("key", 7))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	large := newTestRemote(t, server.URL+"/large", jwks.RemoteOptions{MaxBodyBytes: 64})
	if _, err := large.PublicKey(context.Background(), "key"); !errors.Is(err, jwks.ErrBodyTooLarge) {
		t.Fatalf("large body error = %v", err)
	}
	redirect := newTestRemote(t, server.URL+"/redirect", jwks.RemoteOptions{})
	if _, err := redirect.PublicKey(context.Background(), "key"); err == nil || !strings.Contains(err.Error(), "status 302") {
		t.Fatalf("redirect error = %v", err)
	}
	if got := redirected.Load(); got != 0 {
		t.Fatalf("redirect target requests = %d, want 0", got)
	}
}

func TestRemoteSourceRefreshOutlivesInitiatingCallerWithinBound(t *testing.T) {
	key := testPublicKey("shared", 8)
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		writeSet(t, writer, key)
	}))
	defer server.Close()
	source := newTestRemote(t, server.URL, jwks.RemoteOptions{FetchTimeout: time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, err := source.PublicKey(ctx, "shared")
		firstResult <- err
	}()
	<-started
	secondResult := make(chan error, 1)
	go func() {
		resolved, err := source.PublicKey(context.Background(), "shared")
		if err == nil && !reflect.DeepEqual(resolved, key.Key) {
			err = errors.New("resolved key did not match")
		}
		secondResult <- err
	}()
	cancel()
	if err := <-firstResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("initiating caller error = %v", err)
	}
	close(release)
	if err := <-secondResult; err != nil {
		t.Fatalf("shared waiter error = %v", err)
	}
}

func TestRemoteSourceFetchTimeoutCancelsIssuerRequest(t *testing.T) {
	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
		close(requestCanceled)
	}))
	defer server.Close()
	source := newTestRemote(t, server.URL, jwks.RemoteOptions{FetchTimeout: 30 * time.Millisecond})

	if _, err := source.PublicKey(context.Background(), "key"); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("issuer request context was not canceled")
	}
}

func newTestRemote(t *testing.T, endpoint string, options jwks.RemoteOptions) *jwks.RemoteSource {
	t.Helper()
	options.AllowLoopbackHTTP = true
	source, err := jwks.NewRemoteSource(endpoint, options)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func testPublicKey(keyID string, seed byte) jose.JSONWebKey {
	private := ed25519.NewKeyFromSeed(bytesOf(seed, ed25519.SeedSize))
	return jose.JSONWebKey{Key: private.Public().(ed25519.PublicKey), KeyID: keyID, Use: "sig", Algorithm: "EdDSA"}
}

func testPrivateKey(keyID string, seed byte) jose.JSONWebKey {
	return jose.JSONWebKey{Key: ed25519.NewKeyFromSeed(bytesOf(seed, ed25519.SeedSize)), KeyID: keyID, Use: "sig", Algorithm: "EdDSA"}
}

func bytesOf(value byte, count int) []byte {
	result := make([]byte, count)
	for index := range result {
		result[index] = value
	}
	return result
}

func writeSet(t *testing.T, writer http.ResponseWriter, keys ...jose.JSONWebKey) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/jwk-set+json")
	if err := json.NewEncoder(writer).Encode(jose.JSONWebKeySet{Keys: keys}); err != nil {
		t.Errorf("encode JWKS: %v", err)
	}
}
