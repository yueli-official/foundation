package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestCasbinProjectionMatchesReferenceCandidateVectors(t *testing.T) {
	definition := validDefinition()
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	user := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "user"}
	catalog := authorization.MustCompile(definition)
	reference, err := authorization.NewMemory(catalog, authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() reference error = %v", err)
	}
	projected, err := authorization.NewMemory(catalog, authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() projected error = %v", err)
	}
	ctx := context.Background()
	for _, module := range []*authorization.Memory{reference, projected} {
		if _, err := module.CreateScope(ctx, authorization.CreateScopeCommand{
			Actor: admin, ID: "handbook", Type: "collection", ParentID: "docs",
		}); err != nil {
			t.Fatalf("CreateScope() error = %v", err)
		}
		if _, err := module.CreateScope(ctx, authorization.CreateScopeCommand{
			Actor: admin, ID: "intro", Type: "document", ParentID: "handbook",
		}); err != nil {
			t.Fatalf("CreateScope() child error = %v", err)
		}
		if _, err := module.Grant(ctx, authorization.GrantCommand{
			Actor: admin, Target: user, Role: "author", ScopeID: "handbook",
			Source: authorization.GrantSourceDirect,
		}); err != nil {
			t.Fatalf("Grant() error = %v", err)
		}
	}
	if err := projected.RebuildCasbinProjection(); err != nil {
		t.Fatalf("RebuildCasbinProjection() error = %v", err)
	}
	for _, request := range []authorization.DecisionRequest{
		{Subject: user, Capability: "docs.document.publish", ScopeID: "handbook"},
		{Subject: user, Capability: "docs.document.publish", ScopeID: "intro"},
		{Subject: user, Capability: authorization.CapabilityAuditRead, ScopeID: "intro"},
		{Subject: admin, Capability: authorization.CapabilityAuditRead, ScopeID: "intro"},
	} {
		want, err := reference.Decide(ctx, request)
		if err != nil {
			t.Fatalf("reference.Decide(%#v) error = %v", request, err)
		}
		got, err := projected.Decide(ctx, request)
		if err != nil {
			t.Fatalf("projected.Decide(%#v) error = %v", request, err)
		}
		if got.Allowed != want.Allowed || got.Reason != want.Reason ||
			len(got.Sources) != len(want.Sources) {
			t.Fatalf("projected.Decide(%#v) = %#v, want candidate-equivalent %#v", request, got, want)
		}
	}
}
