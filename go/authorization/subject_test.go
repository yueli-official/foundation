package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestGuestReceivesAuthenticatedLayerButCannotReceiveRoleGrant(t *testing.T) {
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	guest := authorization.SubjectRef{Kind: authorization.SubjectGuest, ID: "guest-session"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}

	decision, err := module.Decide(context.Background(), authorization.DecisionRequest{
		Subject: guest, Capability: authorization.CapabilityApplicationCreate, ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() guest error = %v", err)
	}
	if !decision.Allowed || decision.Reason != authorization.ReasonAccessLayer {
		t.Fatalf("Decide() guest = %#v, want authenticated access-layer allow", decision)
	}
	effective, err := module.EffectiveAccess(context.Background(), authorization.EffectiveAccessQuery{
		Subject: guest, ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("EffectiveAccess() guest error = %v", err)
	}
	if !containsCapability(effective.Capabilities, authorization.CapabilityApplicationCreate) {
		t.Fatalf("EffectiveAccess() capabilities = %v, want application create", effective.Capabilities)
	}

	_, err = module.Grant(context.Background(), authorization.GrantCommand{
		Actor: admin, Target: guest, Role: "author", ScopeID: "docs", Source: authorization.GrantSourceDirect,
	})
	if !authorization.Is(err, authorization.ErrorInvalidInput) {
		t.Fatalf("Grant() guest error = %v, want invalid input", err)
	}
}

func containsCapability(values []authorization.CapabilityKey, want authorization.CapabilityKey) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
