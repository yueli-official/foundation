package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPersonalTokenScopeIsBoundToExactInstance(t *testing.T) {
	scope, err := PersonalScope("blog-production", "blog.post.update")
	if err != nil {
		t.Fatal(err)
	}
	site, capability, ok := ParsePersonalScope(scope)
	if !ok || site != "blog-production" || capability != "blog.post.update" {
		t.Fatal(scope)
	}
	for _, bad := range []string{"site:*:*", "site:YmxvZw:blog.*", "site:YmxvZw==:blog.post.update", "blog.post.update"} {
		if _, _, ok := ParsePersonalScope(bad); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestPersonalTokenRevocationAndScopeRestrictions(t *testing.T) {
	scope, _ := PersonalScope("blog-production", "blog.post.update")
	active := true
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer pat_test" {
			t.Error("wrong credential transport")
		}
		if !active {
			w.WriteHeader(401)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"userKey": "Alice123", "scopes": []string{scope}, "expiresAt": time.Now().Add(time.Minute)})
	}))
	defer server.Close()
	v, err := NewPersonalTokenVerifier(server.URL, "blog-production", nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := v.Verify(context.Background(), "pat_test")
	if err != nil {
		t.Fatal(err)
	}
	ctx := NewContext(context.Background(), p)
	if !p.IsUser() || !p.IsPersonalToken() || !AllowsPersonalCapability(ctx, "blog.post.update") || AllowsPersonalCapability(ctx, "blog.post.publish") {
		t.Fatal("incorrect permission restriction")
	}
	other, _ := NewPersonalTokenVerifier(server.URL, "blog-other", nil)
	if _, err := other.Verify(context.Background(), "pat_test"); !errors.Is(err, ErrInvalidAudience) {
		t.Fatalf("cross-site access: %v", err)
	}
	active = false
	if _, err := v.Verify(context.Background(), "pat_test"); err == nil {
		t.Fatal("revoked token accepted")
	}
	if calls != 3 {
		t.Fatalf("unexpected verification cache: %d", calls)
	}
}

func TestPersonalTokenDoesNotForwardSecretOnRedirect(t *testing.T) {
	called := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer server.Close()
	v, _ := NewPersonalTokenVerifier(server.URL, "blog", nil)
	if _, err := v.Verify(context.Background(), "pat_test"); err == nil || called {
		t.Fatal("credential redirect allowed")
	}
}

func TestPersonalTokenFailsClosedWhenNotConfigured(t *testing.T) {
	if _, err := (CompositeVerifier{}).Verify(context.Background(), "pat_test"); err == nil {
		t.Fatal("PAT accepted without configuration")
	}
	for _, endpoint := range []string{"http://example.com/verify", "https://user:secret@example.com/verify", "https://example.com/verify?token=x"} {
		if _, err := NewPersonalTokenVerifier(endpoint, "blog", nil); err == nil {
			t.Fatalf("unsafe endpoint %s", endpoint)
		}
	}
}
