// Package authorizationtest provides reusable behavioral conformance tests
// for authorization Adapters.
package authorizationtest

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

// Adapter is the management and decision surface required by the shared
// conformance suite. Production consumers should continue to depend on the
// narrower individual interfaces.
type Adapter interface {
	authorization.Authorizer
	authorization.AccessReader
	authorization.ScopeManager
	authorization.ResourceScopeRegistry
	authorization.RoleManager
	authorization.GrantManager
	authorization.GroupManager
	authorization.WorkflowManager
	authorization.PolicyManager
}

type Setup struct {
	Definition        authorization.Definition
	RootScopeID       authorization.ScopeID
	ProtectedSubjects []authorization.SubjectRef
}

type Factory func(context.Context, Setup) (Adapter, func(), error)

// Run executes transport- and storage-neutral authorization behavior. Durable
// Adapters should invoke this suite in addition to their transaction tests.
func Run(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("default deny and scope inheritance", func(t *testing.T) {
		admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
		author := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "author"}
		module := open(t, factory, admin)
		ctx := context.Background()
		command := authorization.RegisterScopeCommand{
			ID: "document-1", Type: "document", ParentID: "site",
		}
		if _, err := module.RegisterScope(ctx, command); err != nil {
			t.Fatalf("RegisterScope() error = %v", err)
		}
		if _, err := module.RegisterScope(ctx, command); err != nil {
			t.Fatalf("RegisterScope() idempotent error = %v", err)
		}
		if _, err := module.CreateScope(ctx, authorization.CreateScopeCommand{
			Actor: admin, ID: "document-2", Type: "document", ParentID: "site",
		}); err != nil {
			t.Fatalf("CreateScope() error = %v", err)
		}
		denied, err := module.Decide(ctx, authorization.DecisionRequest{
			Subject: author, Capability: "content.publish", ScopeID: "document-1",
		})
		if err != nil {
			t.Fatalf("Decide() default error = %v", err)
		}
		if denied.Allowed || denied.Reason != authorization.ReasonNoGrant {
			t.Fatalf("Decide() default = %#v, want no-grant deny", denied)
		}
		if _, err := module.Grant(ctx, authorization.GrantCommand{
			Actor: admin, Target: author, Role: "author", ScopeID: "site",
			Source: authorization.GrantSourceDirect,
		}); err != nil {
			t.Fatalf("Grant() error = %v", err)
		}
		allowed, err := module.Decide(ctx, authorization.DecisionRequest{
			Subject: author, Capability: "content.publish", ScopeID: "document-1",
		})
		if err != nil {
			t.Fatalf("Decide() inherited error = %v", err)
		}
		if !allowed.Allowed || allowed.PolicyRevision != 1 || len(allowed.Sources) != 1 {
			t.Fatalf("Decide() inherited = %#v, want revision-one role allow", allowed)
		}
	})

	t.Run("custom role revision and stable identity", func(t *testing.T) {
		admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
		holder := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "holder"}
		module := open(t, factory, admin)
		ctx := context.Background()
		draft, err := module.CreatePolicyDraft(ctx, authorization.CreatePolicyDraftCommand{
			Actor: admin, ScopeID: "site", ExpectedActiveRevision: 1,
		})
		if err != nil {
			t.Fatalf("CreatePolicyDraft() error = %v", err)
		}
		role, err := module.CreateRole(ctx, authorization.CreateRoleCommand{
			Actor: admin, Revision: draft.Number, Key: "reviewer", DisplayName: "Reviewer", ScopeID: "site",
			Capabilities: []authorization.CapabilityKey{"content.publish"},
			Assignment: authorization.AssignmentPolicy{
				Sources: []authorization.GrantSource{authorization.GrantSourceDirect},
			},
		})
		if err != nil {
			t.Fatalf("CreateRole() error = %v", err)
		}
		if role.ID == "" {
			t.Fatal("CreateRole() returned an empty stable ID")
		}
		if _, err := module.ActivatePolicy(ctx, authorization.ActivatePolicyCommand{
			Actor: admin, Revision: draft.Number, ExpectedActiveRevision: 1,
		}); err != nil {
			t.Fatalf("ActivatePolicy() error = %v", err)
		}
		grant, err := module.Grant(ctx, authorization.GrantCommand{
			Actor: admin, Target: holder, Role: role.Key, ScopeID: "site",
			Source: authorization.GrantSourceDirect,
		})
		if err != nil {
			t.Fatalf("Grant() error = %v", err)
		}
		if grant.RoleID != role.ID {
			t.Fatalf("Grant().RoleID = %q, want %q", grant.RoleID, role.ID)
		}
	})

	t.Run("last protected administrator cannot be revoked", func(t *testing.T) {
		admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
		module := open(t, factory, admin)
		access, err := module.EffectiveAccess(context.Background(), authorization.EffectiveAccessQuery{
			Subject: admin, ScopeID: "site",
		})
		if err != nil {
			t.Fatalf("EffectiveAccess() error = %v", err)
		}
		if len(access.Grants) != 1 {
			t.Fatalf("EffectiveAccess().Grants = %#v, want bootstrap grant", access.Grants)
		}
		_, err = module.Revoke(context.Background(), authorization.RevokeCommand{
			Actor: admin, GrantID: access.Grants[0].ID,
		})
		if !authorization.Is(err, authorization.ErrorInvariant) {
			t.Fatalf("Revoke() last administrator error = %v, want invariant", err)
		}
	})

	t.Run("delegated manager cannot escape its scope", func(t *testing.T) {
		admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
		manager := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "manager"}
		module := open(t, factory, admin)
		ctx := context.Background()
		if _, err := module.CreateScope(ctx, authorization.CreateScopeCommand{
			Actor: admin, ID: "document-1", Type: "document", ParentID: "site",
		}); err != nil {
			t.Fatalf("CreateScope() error = %v", err)
		}
		if _, err := module.Grant(ctx, authorization.GrantCommand{
			Actor: admin, Target: manager, Role: "manager", ScopeID: "document-1",
			Source: authorization.GrantSourceDirect,
		}); err != nil {
			t.Fatalf("Grant() manager error = %v", err)
		}
		_, err := module.Grant(ctx, authorization.GrantCommand{
			Actor:  manager,
			Target: authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "outside"},
			Role:   "author", ScopeID: "site", Source: authorization.GrantSourceDirect,
		})
		if !authorization.Is(err, authorization.ErrorDenied) {
			t.Fatalf("Grant() outside scope error = %v, want denied", err)
		}
	})

	t.Run("groups are flat and workflows have immutable terminal states", func(t *testing.T) {
		admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
		applicant := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "applicant"}
		module := open(t, factory, admin)
		ctx := context.Background()
		if _, err := module.CreateGroup(ctx, authorization.CreateGroupCommand{
			Actor: admin, ID: "editors", ScopeID: "site", DisplayName: "Editors",
		}); err != nil {
			t.Fatalf("CreateGroup() error = %v", err)
		}
		_, err := module.AddGroupMember(ctx, authorization.AddGroupMemberCommand{
			Actor: admin, GroupID: "editors",
			Member: authorization.SubjectRef{Kind: authorization.SubjectGroup, ID: "nested"},
		})
		if !authorization.Is(err, authorization.ErrorInvariant) {
			t.Fatalf("AddGroupMember() nested error = %v, want invariant", err)
		}
		application, err := module.Apply(ctx, authorization.ApplyCommand{
			Actor: applicant, Role: "author", ScopeID: "site", Reason: "I write",
		})
		if err != nil {
			t.Fatalf("Apply() error = %v", err)
		}
		if _, err := module.ReviewApplication(ctx, authorization.ReviewApplicationCommand{
			Actor: admin, ApplicationID: application.ID, Decision: authorization.ReviewApprove,
		}); err != nil {
			t.Fatalf("ReviewApplication() approve error = %v", err)
		}
		_, err = module.ReviewApplication(ctx, authorization.ReviewApplicationCommand{
			Actor: admin, ApplicationID: application.ID, Decision: authorization.ReviewReject,
		})
		if !authorization.Is(err, authorization.ErrorConflict) {
			t.Fatalf("ReviewApplication() terminal rewrite error = %v, want conflict", err)
		}
	})
}

func open(t *testing.T, factory Factory, admin authorization.SubjectRef) Adapter {
	t.Helper()
	setup := Setup{
		Definition:        conformanceDefinition(),
		RootScopeID:       "site",
		ProtectedSubjects: []authorization.SubjectRef{admin},
	}
	module, cleanup, err := factory(context.Background(), setup)
	if err != nil {
		t.Fatalf("Factory() error = %v", err)
	}
	if cleanup != nil {
		t.Cleanup(cleanup)
	}
	return module
}

func conformanceDefinition() authorization.Definition {
	return authorization.Definition{
		Consumer: "conformance",
		Version:  1,
		Capabilities: []authorization.CapabilityDefinition{
			{Key: "content.publish", Version: 1, Binding: authorization.BindingNormal, Delegable: true},
		},
		Scopes: authorization.ScopeSchema{Types: []authorization.ScopeTypeDefinition{
			{Key: "site", Root: true, Children: []authorization.ScopeType{"document"}},
			{Key: "document"},
		}},
		AccessLayers: []authorization.AccessLayerDefinition{
			{Key: authorization.AccessLayerVisitor},
			{Key: authorization.AccessLayerAuthenticated},
		},
		Roles: []authorization.RoleDefinition{
			{
				Key: "administrator", DisplayName: "Administrator", Protected: true,
				Capabilities: []authorization.CapabilityKey{
					authorization.CapabilityManage,
					"content.publish",
				},
			},
			{
				Key: "author", DisplayName: "Author",
				Capabilities: []authorization.CapabilityKey{"content.publish"},
				Assignment: authorization.AssignmentPolicy{
					Sources: []authorization.GrantSource{
						authorization.GrantSourceApplication,
						authorization.GrantSourceDirect,
						authorization.GrantSourceGroup,
					},
				},
			},
			{
				Key: "manager", DisplayName: "Manager",
				Capabilities: []authorization.CapabilityKey{
					authorization.CapabilityManage,
					"content.publish",
				},
				Assignment: authorization.AssignmentPolicy{
					Sources: []authorization.GrantSource{authorization.GrantSourceDirect},
				},
			},
		},
	}
}
