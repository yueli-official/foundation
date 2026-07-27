package httpadapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestClientCredentialsTokenSourceCachesToken(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		user, password, ok := request.BasicAuth()
		if !ok || user != "identity-privacy" || password != "secret" {
			t.Fatal("missing client authentication")
		}
		if request.FormValue("grant_type") != "client_credentials" || request.FormValue("scope") != "privacy:owner" {
			t.Fatal("wrong token request")
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"access_token": "token-1", "expires_in": 300,
		})
	}))
	defer server.Close()
	source := &ClientCredentialsTokenSource{
		TokenURL: server.URL, ClientID: "identity-privacy", ClientSecret: "secret", Scope: "privacy:owner",
	}
	for range 2 {
		token, err := source.Token(context.Background())
		if err != nil || token != "token-1" {
			t.Fatalf("token = %q, %v", token, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("token endpoint calls = %d, want 1", calls.Load())
	}
}
