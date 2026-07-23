package turnstile_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yueli-official/foundation/go/abuse"
	"github.com/yueli-official/foundation/go/abuse/turnstile"
)

func TestVerifierRetriesWithTheSameIdempotencyKey(t *testing.T) {
	var (
		mu    sync.Mutex
		keys  []string
		calls int
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		keys = append(keys, request.Form.Get("idempotency_key"))
		calls++
		call := calls
		mu.Unlock()
		if call == 1 {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(writer, `{"success":true,"hostname":"example.test","action":"login"}`)
	}))
	defer server.Close()
	verifier, err := turnstile.New(turnstile.Options{
		Secret: "secret", Endpoint: server.URL, MaxAttempts: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	verification, err := verifier.Verify(context.Background(), abuse.VerificationRequest{
		VerificationID: "2edca36d-ea3b-5d24-9dd7-f5cd60fca3e5",
		Token:          "proof", ExpectedAction: "login", AllowedHosts: []string{"example.test"},
	})
	if err != nil || verification.Status != abuse.VerificationAccepted {
		t.Fatalf("verification=%+v err=%v", verification, err)
	}
	if len(keys) != 2 || keys[0] != keys[1] {
		t.Fatalf("idempotency keys=%v", keys)
	}
}

func TestVerifierRejectsMismatchAndOversizedProofWithoutLeaking(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = io.WriteString(writer, `{"success":true,"hostname":"evil.test","action":"other"}`)
	}))
	defer server.Close()
	verifier, err := turnstile.New(turnstile.Options{
		Secret: "top-secret-value", Endpoint: server.URL, MaxAttempts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	verification, err := verifier.Verify(context.Background(), abuse.VerificationRequest{
		VerificationID: "2edca36d-ea3b-5d24-9dd7-f5cd60fca3e5",
		Token:          "sensitive-proof", ExpectedAction: "login", AllowedHosts: []string{"example.test"},
	})
	if err != nil || verification.Status != abuse.VerificationRejected {
		t.Fatalf("verification=%+v err=%v", verification, err)
	}
	verification, err = verifier.Verify(context.Background(), abuse.VerificationRequest{
		VerificationID: "2edca36d-ea3b-5d24-9dd7-f5cd60fca3e5",
		Token:          strings.Repeat("x", turnstile.MaxTokenBytes+1),
		ExpectedAction: "login", AllowedHosts: []string{"example.test"},
	})
	if err != nil || verification.Status != abuse.VerificationRejected || calls != 1 {
		t.Fatalf("oversized verification=%+v calls=%d err=%v", verification, calls, err)
	}
}

func TestVerifierOperationalErrorRedactsSecretAndProof(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	server.Close()
	verifier, err := turnstile.New(turnstile.Options{
		Secret: "top-secret-value", Endpoint: server.URL, MaxAttempts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = verifier.Verify(context.Background(), abuse.VerificationRequest{
		VerificationID: "2edca36d-ea3b-5d24-9dd7-f5cd60fca3e5",
		Token:          "sensitive-proof", ExpectedAction: "login", AllowedHosts: []string{"example.test"},
	})
	if !abuse.IsKind(err, abuse.ErrorVerifierUnavailable) {
		t.Fatalf("error=%v", err)
	}
	message := err.Error()
	if strings.Contains(message, "top-secret-value") || strings.Contains(message, "sensitive-proof") {
		t.Fatalf("error leaked sensitive data: %s", message)
	}
}
