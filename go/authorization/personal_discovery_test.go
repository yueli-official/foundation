package authorization_test

import (
	"context"
	"slices"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestDescendantDiscoveryDoesNotGrantSiblingAccess(t *testing.T) {
	d := validDefinition()
	d.Capabilities[0].AllowedScopes = []authorization.ScopeType{"document"}
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	user := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "writer"}
	m, err := authorization.NewMemory(authorization.MustCompile(d), authorization.MemoryOptions{RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, id := range []authorization.ScopeID{"own", "other"} {
		if _, err := m.CreateScope(ctx, authorization.CreateScopeCommand{Actor: admin, ID: id, Type: "collection", ParentID: "docs"}); err != nil {
			t.Fatal(err)
		}
		if _, err := m.CreateScope(ctx, authorization.CreateScopeCommand{Actor: admin, ID: id + "-doc", Type: "document", ParentID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Grant(ctx, authorization.GrantCommand{Actor: admin, Target: user, Role: "author", ScopeID: "own", Source: authorization.GrantSourceDirect}); err != nil {
		t.Fatal(err)
	}
	for _, include := range []bool{false, true} {
		access, err := m.EffectiveAccess(ctx, authorization.EffectiveAccessQuery{Subject: user, ScopeID: "docs", IncludeDescendants: include})
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(access.Capabilities, "docs.document.publish") != include {
			t.Fatalf("include=%v access=%+v", include, access)
		}
	}
	for _, id := range []authorization.ScopeID{"own-doc", "other-doc"} {
		decision, err := m.Decide(ctx, authorization.DecisionRequest{Subject: user, Capability: "docs.document.publish", ScopeID: id})
		if err != nil {
			t.Fatal(err)
		}
		if decision.Allowed != (id == "own-doc") {
			t.Fatalf("scope=%s decision=%+v", id, decision)
		}
	}
}
