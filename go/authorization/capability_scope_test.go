package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestCapabilityCanOnlyBeCheckedAtDeclaredScopeTypes(t *testing.T) {
	definition := validDefinition()
	definition.Capabilities[0].AllowedScopes = []authorization.ScopeType{"document"}
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}

	_, err = module.Decide(context.Background(), authorization.DecisionRequest{
		Subject: admin, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if !authorization.Is(err, authorization.ErrorInvalidInput) {
		t.Fatalf("Decide() at root error = %v, want invalid input", err)
	}
}
