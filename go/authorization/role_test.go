package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestScopedCustomRoleActivatesWithStableIDAndCannotEscapeItsScope(t *testing.T) {
	definition := validDefinition()
	definition.Capabilities[0].Delegable = true
	definition.Roles = append(definition.Roles, authorization.RoleDefinition{
		Key: "collection_manager", DisplayName: "文档集管理员",
		Capabilities: []authorization.CapabilityKey{
			authorization.CapabilityManage,
			"docs.document.publish",
		},
		Assignment: authorization.AssignmentPolicy{
			Sources: []authorization.GrantSource{authorization.GrantSourceDirect},
		},
	})
	ctx := context.Background()
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	manager := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "manager"}
	reviewer := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "reviewer"}
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	if _, err := module.CreateScope(ctx, authorization.CreateScopeCommand{
		Actor: admin, ID: "handbook", Type: "collection", ParentID: "docs",
	}); err != nil {
		t.Fatalf("CreateScope() collection error = %v", err)
	}
	if _, err := module.CreateScope(ctx, authorization.CreateScopeCommand{
		Actor: admin, ID: "intro", Type: "document", ParentID: "handbook",
	}); err != nil {
		t.Fatalf("CreateScope() document error = %v", err)
	}
	if _, err := module.Grant(ctx, authorization.GrantCommand{
		Actor: admin, Target: manager, Role: "collection_manager", ScopeID: "handbook",
		Source: authorization.GrantSourceDirect,
	}); err != nil {
		t.Fatalf("Grant() manager error = %v", err)
	}

	draft, err := module.CreatePolicyDraft(ctx, authorization.CreatePolicyDraftCommand{
		Actor: manager, ScopeID: "handbook", ExpectedActiveRevision: 1,
	})
	if err != nil {
		t.Fatalf("CreatePolicyDraft() error = %v", err)
	}
	created, err := module.CreateRole(ctx, authorization.CreateRoleCommand{
		Actor: manager, Revision: draft.Number, Key: "reviewer", DisplayName: "审阅者",
		ScopeID:      "handbook",
		Capabilities: []authorization.CapabilityKey{"docs.document.publish"},
		Assignment: authorization.AssignmentPolicy{
			Sources: []authorization.GrantSource{authorization.GrantSourceDirect},
		},
	})
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	if created.ID == "" || created.Kind != authorization.RoleCustom {
		t.Fatalf("CreateRole() = %#v, want custom role with stable ID", created)
	}
	if _, err := module.Grant(ctx, authorization.GrantCommand{
		Actor: manager, Target: reviewer, Role: "reviewer", ScopeID: "handbook",
		Source: authorization.GrantSourceDirect,
	}); !authorization.Is(err, authorization.ErrorInvalidInput) {
		t.Fatalf("Grant() before activation error = %v, want invalid input", err)
	}
	if _, err := module.ActivatePolicy(ctx, authorization.ActivatePolicyCommand{
		Actor: manager, Revision: draft.Number, ExpectedActiveRevision: 1,
	}); err != nil {
		t.Fatalf("ActivatePolicy() error = %v", err)
	}
	grant, err := module.Grant(ctx, authorization.GrantCommand{
		Actor: manager, Target: reviewer, Role: "reviewer", ScopeID: "handbook",
		Source: authorization.GrantSourceDirect,
	})
	if err != nil {
		t.Fatalf("Grant() reviewer error = %v", err)
	}
	if grant.RoleID != created.ID {
		t.Fatalf("Grant() RoleID = %q, want %q", grant.RoleID, created.ID)
	}
	decision, err := module.Decide(ctx, authorization.DecisionRequest{
		Subject: reviewer, Capability: "docs.document.publish", ScopeID: "intro",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("Decide() = %#v, want custom-role allow", decision)
	}

	_, err = module.Grant(ctx, authorization.GrantCommand{
		Actor: manager, Target: authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "outside"},
		Role: "reviewer", ScopeID: "docs", Source: authorization.GrantSourceDirect,
	})
	if !authorization.Is(err, authorization.ErrorDenied) {
		t.Fatalf("Grant() outside custom role scope error = %v, want denied", err)
	}
}

func TestCustomRoleWithActiveGrantCannotBeSilentlyRetired(t *testing.T) {
	// The complete lifecycle is covered by the first tracer; this assertion
	// fixes the destructive edge after activation.
	definition := validDefinition()
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	draft, err := module.CreatePolicyDraft(context.Background(), authorization.CreatePolicyDraftCommand{
		Actor: admin, ScopeID: "docs", ExpectedActiveRevision: 1,
	})
	if err != nil {
		t.Fatalf("CreatePolicyDraft() error = %v", err)
	}
	_, err = module.CreateRole(context.Background(), authorization.CreateRoleCommand{
		Actor: admin, Revision: draft.Number, Key: "temporary", DisplayName: "临时角色", ScopeID: "docs",
		Capabilities: []authorization.CapabilityKey{"docs.document.publish"},
		Assignment: authorization.AssignmentPolicy{
			Sources: []authorization.GrantSource{authorization.GrantSourceDirect},
		},
	})
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	if _, err := module.ActivatePolicy(context.Background(), authorization.ActivatePolicyCommand{
		Actor: admin, Revision: draft.Number, ExpectedActiveRevision: 1,
	}); err != nil {
		t.Fatalf("ActivatePolicy() error = %v", err)
	}
	if _, err := module.Grant(context.Background(), authorization.GrantCommand{
		Actor: admin, Target: authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "holder"},
		Role: "temporary", ScopeID: "docs", Source: authorization.GrantSourceDirect,
	}); err != nil {
		t.Fatalf("Grant() temporary error = %v", err)
	}
	retireDraft, err := module.CreatePolicyDraft(context.Background(), authorization.CreatePolicyDraftCommand{
		Actor: admin, ScopeID: "docs", ExpectedActiveRevision: 2,
	})
	if err != nil {
		t.Fatalf("CreatePolicyDraft() retire error = %v", err)
	}
	_, err = module.RetireRole(context.Background(), authorization.RetireRoleCommand{
		Actor: admin, Revision: retireDraft.Number, Role: "temporary",
	})
	if !authorization.Is(err, authorization.ErrorConflict) {
		t.Fatalf("RetireRole() error = %v, want active-reference conflict", err)
	}
}

func TestSourceConstraintCanCoverPresentAndFutureNormalRoles(t *testing.T) {
	definition := validDefinition()
	definition.Constraints = []authorization.ConstraintDefinition{
		{
			Key: "docs.normal_role_owns_document", Version: 1, Mode: authorization.ConstraintSource,
			Capabilities:   []authorization.CapabilityKey{"docs.document.publish"},
			AllNormalRoles: true,
		},
	}
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	reviewer := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "reviewer"}
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
		Constraints: map[authorization.ConstraintKey]authorization.ConstraintEvaluator{
			"docs.normal_role_owns_document": authorization.ConstraintFunc(func(_ context.Context, input authorization.ConstraintInput) authorization.ConstraintResult {
				return authorization.ConstraintResult{Denied: true}
			}),
		},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	draft, err := module.CreatePolicyDraft(context.Background(), authorization.CreatePolicyDraftCommand{
		Actor: admin, ScopeID: "docs", ExpectedActiveRevision: 1,
	})
	if err != nil {
		t.Fatalf("CreatePolicyDraft() error = %v", err)
	}
	if _, err := module.CreateRole(context.Background(), authorization.CreateRoleCommand{
		Actor: admin, Revision: draft.Number, Key: "reviewer", DisplayName: "审阅者", ScopeID: "docs",
		Capabilities: []authorization.CapabilityKey{"docs.document.publish"},
		Assignment: authorization.AssignmentPolicy{
			Sources: []authorization.GrantSource{authorization.GrantSourceDirect},
		},
	}); err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	if _, err := module.ActivatePolicy(context.Background(), authorization.ActivatePolicyCommand{
		Actor: admin, Revision: draft.Number, ExpectedActiveRevision: 1,
	}); err != nil {
		t.Fatalf("ActivatePolicy() error = %v", err)
	}
	if _, err := module.Grant(context.Background(), authorization.GrantCommand{
		Actor: admin, Target: reviewer, Role: "reviewer", ScopeID: "docs",
		Source: authorization.GrantSourceDirect,
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}

	decision, err := module.Decide(context.Background(), authorization.DecisionRequest{
		Subject: reviewer, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Allowed || decision.Constraint != "docs.normal_role_owns_document" {
		t.Fatalf("Decide() = %#v, want future normal role covered by constraint", decision)
	}

	adminDecision, err := module.Decide(context.Background(), authorization.DecisionRequest{
		Subject: admin, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() administrator error = %v", err)
	}
	if !adminDecision.Allowed {
		t.Fatalf("Decide() administrator = %#v, want protected source excluded", adminDecision)
	}
}

func TestCustomRoleUpdateAndRetirementPreserveIdentity(t *testing.T) {
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	draft, err := module.CreatePolicyDraft(context.Background(), authorization.CreatePolicyDraftCommand{
		Actor: admin, ScopeID: "docs", ExpectedActiveRevision: 1,
	})
	if err != nil {
		t.Fatalf("CreatePolicyDraft() error = %v", err)
	}
	created, err := module.CreateRole(context.Background(), authorization.CreateRoleCommand{
		Actor: admin, Revision: draft.Number, Key: "reviewer", DisplayName: "审阅者", ScopeID: "docs",
		Capabilities: []authorization.CapabilityKey{"docs.document.publish"},
		Assignment: authorization.AssignmentPolicy{
			Sources: []authorization.GrantSource{authorization.GrantSourceDirect},
		},
	})
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	updated, err := module.UpdateRole(context.Background(), authorization.UpdateRoleCommand{
		Actor: admin, Revision: draft.Number, Role: "reviewer", DisplayName: "高级审阅者",
		Capabilities: []authorization.CapabilityKey{"docs.document.publish"},
		Assignment: authorization.AssignmentPolicy{
			Sources: []authorization.GrantSource{
				authorization.GrantSourceApplication,
				authorization.GrantSourceDirect,
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}
	if updated.ID != created.ID || updated.ScopeID != created.ScopeID || updated.DisplayName != "高级审阅者" {
		t.Fatalf("UpdateRole() = %#v, want stable identity and immutable scope", updated)
	}
	if _, err := module.ActivatePolicy(context.Background(), authorization.ActivatePolicyCommand{
		Actor: admin, Revision: draft.Number, ExpectedActiveRevision: 1,
	}); err != nil {
		t.Fatalf("ActivatePolicy() error = %v", err)
	}

	retireDraft, err := module.CreatePolicyDraft(context.Background(), authorization.CreatePolicyDraftCommand{
		Actor: admin, ScopeID: "docs", ExpectedActiveRevision: 2,
	})
	if err != nil {
		t.Fatalf("CreatePolicyDraft() retirement error = %v", err)
	}
	retired, err := module.RetireRole(context.Background(), authorization.RetireRoleCommand{
		Actor: admin, Revision: retireDraft.Number, Role: "reviewer",
	})
	if err != nil {
		t.Fatalf("RetireRole() error = %v", err)
	}
	if retired.ID != created.ID || retired.Status != authorization.RoleRetired {
		t.Fatalf("RetireRole() = %#v, want stable retired role", retired)
	}
	if _, err := module.ActivatePolicy(context.Background(), authorization.ActivatePolicyCommand{
		Actor: admin, Revision: retireDraft.Number, ExpectedActiveRevision: 2,
	}); err != nil {
		t.Fatalf("ActivatePolicy() retirement error = %v", err)
	}
	_, err = module.Grant(context.Background(), authorization.GrantCommand{
		Actor: admin, Target: authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "reviewer"},
		Role: "reviewer", ScopeID: "docs", Source: authorization.GrantSourceDirect,
	})
	if !authorization.Is(err, authorization.ErrorInvalidInput) {
		t.Fatalf("Grant() retired role error = %v, want invalid input", err)
	}
}
