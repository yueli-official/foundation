package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestPolicyRevisionAtomicallyChangesRoleBindingsAndCanRollback(t *testing.T) {
	ctx := context.Background()
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	author := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "author"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	if _, err := module.Grant(ctx, authorization.GrantCommand{
		Actor: admin, Target: author, Role: "author", ScopeID: "docs", Source: authorization.GrantSourceDirect,
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}

	draft, err := module.CreatePolicyDraft(ctx, authorization.CreatePolicyDraftCommand{
		Actor: admin, ExpectedActiveRevision: 1,
	})
	if err != nil {
		t.Fatalf("CreatePolicyDraft() error = %v", err)
	}
	if _, err := module.SetRoleCapabilities(ctx, authorization.SetRoleCapabilitiesCommand{
		Actor: admin, Revision: draft.Number, Role: "author",
		Capabilities: []authorization.CapabilityKey{},
	}); err != nil {
		t.Fatalf("SetRoleCapabilities() error = %v", err)
	}
	impact, err := module.PreviewPolicy(ctx, authorization.PreviewPolicyCommand{
		Actor: admin, Revision: draft.Number,
	})
	if err != nil {
		t.Fatalf("PreviewPolicy() error = %v", err)
	}
	if impact.RemovedBindings != 1 {
		t.Fatalf("PreviewPolicy() = %#v, want one removed binding", impact)
	}
	active, err := module.ActivatePolicy(ctx, authorization.ActivatePolicyCommand{
		Actor: admin, Revision: draft.Number, ExpectedActiveRevision: 1,
	})
	if err != nil {
		t.Fatalf("ActivatePolicy() error = %v", err)
	}
	if active.Number != 2 || active.State != authorization.PolicyActive {
		t.Fatalf("ActivatePolicy() = %#v, want active revision 2", active)
	}
	denied, err := module.Decide(ctx, authorization.DecisionRequest{
		Subject: author, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() after activation error = %v", err)
	}
	if denied.Allowed || denied.PolicyRevision != 2 {
		t.Fatalf("Decide() after activation = %#v, want revision-2 deny", denied)
	}

	rolledBack, err := module.RollbackPolicy(ctx, authorization.RollbackPolicyCommand{
		Actor: admin, SourceRevision: 1, ExpectedActiveRevision: 2,
	})
	if err != nil {
		t.Fatalf("RollbackPolicy() error = %v", err)
	}
	if rolledBack.Number != 3 || rolledBack.State != authorization.PolicyActive {
		t.Fatalf("RollbackPolicy() = %#v, want active revision 3", rolledBack)
	}
	allowed, err := module.Decide(ctx, authorization.DecisionRequest{
		Subject: author, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() after rollback error = %v", err)
	}
	if !allowed.Allowed || allowed.PolicyRevision != 3 {
		t.Fatalf("Decide() after rollback = %#v, want revision-3 allow", allowed)
	}
}

func TestPolicyCannotWeakenProtectedRole(t *testing.T) {
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	draft, err := module.CreatePolicyDraft(context.Background(), authorization.CreatePolicyDraftCommand{
		Actor: admin, ExpectedActiveRevision: 1,
	})
	if err != nil {
		t.Fatalf("CreatePolicyDraft() error = %v", err)
	}
	_, err = module.SetRoleCapabilities(context.Background(), authorization.SetRoleCapabilitiesCommand{
		Actor: admin, Revision: draft.Number, Role: "administrator",
		Capabilities: []authorization.CapabilityKey{authorization.CapabilityManage},
	})
	if !authorization.Is(err, authorization.ErrorInvariant) {
		t.Fatalf("SetRoleCapabilities() protected role error = %v, want invariant violation", err)
	}
}
