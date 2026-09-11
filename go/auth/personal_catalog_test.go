package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPersonalCatalogRejectsMismatchedAuthorityResponse(t *testing.T) {
	for _, test := range []struct {
		name   string
		result PersonalPermissions
	}{
		{"wrong user", PersonalPermissions{Site: "blog", UserKey: "another-user", Items: []PersonalPermission{}}},
		{"wrong site", PersonalPermissions{Site: "another-blog", UserKey: "user", Items: []PersonalPermission{}}},
		{"wildcard", PersonalPermissions{Site: "blog", UserKey: "user", Items: []PersonalPermission{{Key: "*", Label: "All"}}}},
		{"duplicate", PersonalPermissions{Site: "blog", UserKey: "user", Items: []PersonalPermission{{Key: "post.read", Label: "Read"}, {Key: "post.read", Label: "Read"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(test.result) }))
			defer server.Close()
			catalog, err := NewPersonalCatalog([]PersonalApplication{{ID: "blog", Name: "Blog", PermissionsURL: server.URL, Audience: "blog-api"}}, func(context.Context, string) (string, error) { return "service-proof", nil }, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := catalog.Permissions(context.Background(), "blog", "user"); err == nil {
				t.Fatal("accepted invalid authority response")
			}
		})
	}
}

func TestPersonalCatalogUsesOnlyTrustedDestination(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer server.Close()
	catalog, err := NewPersonalCatalog([]PersonalApplication{{ID: "blog", Name: "Blog", PermissionsURL: server.URL, Audience: "blog-api"}}, func(context.Context, string) (string, error) { return "service-proof", nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Permissions(context.Background(), server.URL, "user"); err == nil || called {
		t.Fatal("user-controlled target accepted")
	}
}
