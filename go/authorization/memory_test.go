package authorization_test

import (
	"context"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestMemoryGrantDrivesScopedDecisions(t *testing.T) {
	now := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	author := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "author"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID:       "docs",
		ProtectedSubjects: []authorization.SubjectRef{admin},
		Clock:             func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	ctx := context.Background()
	collection, err := module.CreateScope(ctx, authorization.CreateScopeCommand{
		Actor: admin, ID: "handbook", Type: "collection", ParentID: "docs",
	})
	if err != nil {
		t.Fatalf("CreateScope() error = %v", err)
	}

	before, err := module.Decide(ctx, authorization.DecisionRequest{
		Subject: author, Capability: "docs.document.publish", ScopeID: collection.ID,
	})
	if err != nil {
		t.Fatalf("Decide() before grant error = %v", err)
	}
	if before.Allowed || before.Reason != authorization.ReasonNoGrant {
		t.Fatalf("Decide() before grant = %#v, want no-grant deny", before)
	}

	grant, err := module.Grant(ctx, authorization.GrantCommand{
		Actor: admin, Target: author, Role: "author", ScopeID: "docs",
		Source: authorization.GrantSourceDirect,
	})
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	allowed, err := module.Decide(ctx, authorization.DecisionRequest{
		Subject: author, Capability: "docs.document.publish", ScopeID: collection.ID,
	})
	if err != nil {
		t.Fatalf("Decide() after grant error = %v", err)
	}
	if !allowed.Allowed || allowed.Reason != authorization.ReasonRoleGrant {
		t.Fatalf("Decide() after grant = %#v, want role-grant allow", allowed)
	}
	if len(allowed.Sources) != 1 || allowed.Sources[0].GrantID != grant.ID {
		t.Fatalf("Decide() sources = %#v, want grant %q", allowed.Sources, grant.ID)
	}

	now = now.Add(2 * time.Hour)
	expiring, err := module.Grant(ctx, authorization.GrantCommand{
		Actor: admin, Target: authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "temporary"},
		Role: "author", ScopeID: "docs", Source: authorization.GrantSourceDirect,
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Grant() expiring error = %v", err)
	}
	now = expiring.ExpiresAt.Add(time.Nanosecond)
	expired, err := module.Decide(ctx, authorization.DecisionRequest{
		Subject: expiring.Target, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() expired error = %v", err)
	}
	if expired.Allowed || expired.Reason != authorization.ReasonNoGrant {
		t.Fatalf("Decide() expired = %#v, want no-grant deny", expired)
	}
}

func TestMemoryWillNotRevokeTheLastProtectedAdministrator(t *testing.T) {
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID:       "docs",
		ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	grants, err := module.EffectiveAccess(context.Background(), authorization.EffectiveAccessQuery{
		Subject: admin, ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("EffectiveAccess() error = %v", err)
	}
	if len(grants.Grants) != 1 {
		t.Fatalf("EffectiveAccess() grants = %d, want 1", len(grants.Grants))
	}

	_, err = module.Revoke(context.Background(), authorization.RevokeCommand{
		Actor: admin, GrantID: grants.Grants[0].ID,
	})
	if !authorization.Is(err, authorization.ErrorInvariant) {
		t.Fatalf("Revoke() error = %v, want invariant violation", err)
	}
}
