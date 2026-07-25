package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestResourceScopeRegistryIsIdempotentAndNeverGrantsAccess(t *testing.T) {
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	author := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "author"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	command := authorization.RegisterScopeCommand{ID: "guide", Type: "collection", ParentID: "docs"}
	first, err := module.RegisterScope(context.Background(), command)
	if err != nil {
		t.Fatalf("RegisterScope() error = %v", err)
	}
	second, err := module.RegisterScope(context.Background(), command)
	if err != nil || second != first {
		t.Fatalf("RegisterScope() repeat = %#v, %v; want %#v", second, err, first)
	}
	if _, err := module.RegisterScope(context.Background(), authorization.RegisterScopeCommand{
		ID: "guide", Type: "document", ParentID: "docs",
	}); !authorization.Is(err, authorization.ErrorConflict) {
		t.Fatalf("RegisterScope() shape conflict error = %v, want conflict", err)
	}
	decision, err := module.Decide(context.Background(), authorization.DecisionRequest{
		Subject: author, Capability: "docs.document.publish", ScopeID: "guide",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Allowed {
		t.Fatal("RegisterScope() unexpectedly granted access")
	}
}

func TestResourceScopeRegistryReparentsExistingScope(t *testing.T) {
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	ctx := context.Background()
	for _, command := range []authorization.RegisterScopeCommand{
		{ID: "first", Type: "collection", ParentID: "docs"},
		{ID: "second", Type: "collection", ParentID: "docs"},
		{ID: "article", Type: "document", ParentID: "first"},
	} {
		if _, err := module.RegisterScope(ctx, command); err != nil {
			t.Fatalf("RegisterScope(%q) error = %v", command.ID, err)
		}
	}
	moved, err := module.ReparentScope(ctx, authorization.ReparentScopeCommand{
		ID: "article", ParentID: "second",
	})
	if err != nil {
		t.Fatalf("ReparentScope() error = %v", err)
	}
	if moved.ParentID != "second" {
		t.Fatalf("ReparentScope().ParentID = %q, want second", moved.ParentID)
	}
	repeated, err := module.ReparentScope(ctx, authorization.ReparentScopeCommand{
		ID: "article", ParentID: "second",
	})
	if err != nil || repeated != moved {
		t.Fatalf("ReparentScope() repeat = %#v, %v; want %#v", repeated, err, moved)
	}
	if _, err := module.ReparentScope(ctx, authorization.ReparentScopeCommand{
		ID: "docs", ParentID: "first",
	}); !authorization.Is(err, authorization.ErrorInvariant) {
		t.Fatalf("ReparentScope(root) error = %v, want invariant", err)
	}
}
