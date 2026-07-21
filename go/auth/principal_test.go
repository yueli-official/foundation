package auth_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/auth"
)

func TestPrincipalNilHelpersAreSafe(t *testing.T) {
	var principal *auth.Principal
	if principal.HasScope("scope") || principal.HasRole("role") || principal.ActorKey() != "" {
		t.Fatal("nil Principal helpers must return zero values")
	}
	if _, ok := principal.Claim("claim"); ok {
		t.Fatal("nil Principal returned a claim")
	}
}

func TestPrincipalActorKeyFallsBackToClient(t *testing.T) {
	principal := &auth.Principal{ClientID: "machine-client"}
	if got := principal.ActorKey(); got != "machine-client" {
		t.Fatalf("ActorKey() = %q", got)
	}
}

func TestPrincipalContextRoundTrip(t *testing.T) {
	principal := &auth.Principal{Subject: "user"}
	ctx := auth.NewContext(context.Background(), principal)
	resolved, ok := auth.FromContext(ctx)
	if !ok || resolved != principal {
		t.Fatalf("FromContext() = %#v, %v", resolved, ok)
	}
	if _, ok := auth.FromContext(nil); ok {
		t.Fatal("FromContext(nil) found a Principal")
	}
}
