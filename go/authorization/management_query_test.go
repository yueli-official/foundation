package authorization_test

import (
	"context"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestManagementQueriesSeparateRequestableRolesOwnWorkflowAndManagedState(t *testing.T) {
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	applicant := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "applicant"}
	other := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "other"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	ctx := context.Background()
	roles, err := module.ListRoles(ctx, authorization.RoleListQuery{
		Actor: admin, ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	if roles.Total != 2 || roles.Roles[0].Key != "administrator" || roles.Roles[1].Key != "author" {
		t.Fatalf("ListRoles() = %#v, want sorted built-in roles", roles)
	}
	if _, err := module.ListRoles(ctx, authorization.RoleListQuery{
		Actor: applicant, ScopeID: "docs",
	}); !authorization.Is(err, authorization.ErrorDenied) {
		t.Fatalf("ListRoles() ordinary error = %v, want denied", err)
	}
	requestable, err := module.ListRequestableRoles(ctx, authorization.RequestableRoleQuery{
		Subject: applicant, ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("ListRequestableRoles() error = %v", err)
	}
	if len(requestable) != 1 || requestable[0].Key != "author" {
		t.Fatalf("ListRequestableRoles() = %#v, want author only", requestable)
	}
	application, err := module.Apply(ctx, authorization.ApplyCommand{
		Actor: applicant, Role: "author", ScopeID: "docs", Reason: "write",
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	own, err := module.ListApplications(ctx, authorization.ApplicationListQuery{
		Actor: applicant, Subject: applicant,
	})
	if err != nil {
		t.Fatalf("ListApplications() own error = %v", err)
	}
	if own.Total != 1 || own.Applications[0].ID != application.ID {
		t.Fatalf("ListApplications() own = %#v, want application", own)
	}
	if _, err := module.ListApplications(ctx, authorization.ApplicationListQuery{
		Actor: other, Subject: applicant,
	}); !authorization.Is(err, authorization.ErrorDenied) {
		t.Fatalf("ListApplications() other error = %v, want denied", err)
	}
	managed, err := module.ListApplications(ctx, authorization.ApplicationListQuery{
		Actor: admin, ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("ListApplications() managed error = %v", err)
	}
	if managed.Total != 1 {
		t.Fatalf("ListApplications() managed = %#v, want one", managed)
	}
}

func TestManagedReadersExposeScopedStateWithoutDirectStoreAccess(t *testing.T) {
	definition := validDefinition()
	definition.Roles[1].Assignment.Sources = append(
		definition.Roles[1].Assignment.Sources,
		authorization.GrantSourceInvitation,
	)
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	author := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "author"}
	clock := func() time.Time { return time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC) }
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin}, Clock: clock,
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	ctx := context.Background()
	if _, err := module.CreateScope(ctx, authorization.CreateScopeCommand{
		Actor: admin, ID: "handbook", Type: "collection", ParentID: "docs",
	}); err != nil {
		t.Fatalf("CreateScope() error = %v", err)
	}
	if _, err := module.CreateGroup(ctx, authorization.CreateGroupCommand{
		Actor: admin, ID: "editors", ScopeID: "docs", DisplayName: "Editors",
	}); err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}
	if _, err := module.Grant(ctx, authorization.GrantCommand{
		Actor: admin, Target: author, Role: "author", ScopeID: "docs",
		Source: authorization.GrantSourceDirect,
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	if _, err := module.Invite(ctx, authorization.InviteCommand{
		Actor: admin, Subject: authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "invitee"},
		Role: "author", ScopeID: "docs", ExpiresAt: clock().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Invite() error = %v", err)
	}
	scopes, err := module.ListScopes(ctx, authorization.ScopeListQuery{Actor: admin, ScopeID: "docs"})
	if err != nil || scopes.Total != 2 {
		t.Fatalf("ListScopes() = %#v, %v, want root and child", scopes, err)
	}
	grants, err := module.ListGrants(ctx, authorization.GrantListQuery{Actor: admin, ScopeID: "docs"})
	if err != nil || grants.Total != 2 {
		t.Fatalf("ListGrants() = %#v, %v, want bootstrap and author", grants, err)
	}
	groups, err := module.ListGroups(ctx, authorization.GroupListQuery{Actor: admin, ScopeID: "docs"})
	if err != nil || groups.Total != 1 {
		t.Fatalf("ListGroups() = %#v, %v, want group", groups, err)
	}
	invitations, err := module.ListInvitations(ctx, authorization.InvitationListQuery{Actor: admin, ScopeID: "docs"})
	if err != nil || invitations.Total != 1 {
		t.Fatalf("ListInvitations() = %#v, %v, want invitation", invitations, err)
	}
	policies, err := module.ListPolicyRevisions(ctx, authorization.PolicyRevisionListQuery{
		Actor: admin, ScopeID: "docs",
	})
	if err != nil || policies.Total != 1 || policies.Revisions[0].State != authorization.PolicyActive {
		t.Fatalf("ListPolicyRevisions() = %#v, %v, want active revision", policies, err)
	}
	snapshot, err := module.GetPolicySnapshot(ctx, authorization.PolicySnapshotQuery{
		Actor: admin, Revision: policies.Revisions[0].Number, ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("GetPolicySnapshot() error = %v", err)
	}
	if len(snapshot.Roles) != 2 || snapshot.Roles[1].Key != "author" {
		t.Fatalf("GetPolicySnapshot().Roles = %#v, want built-in roles", snapshot.Roles)
	}
	if _, err := module.GetPolicySnapshot(ctx, authorization.PolicySnapshotQuery{
		Actor: author, Revision: policies.Revisions[0].Number, ScopeID: "docs",
	}); !authorization.Is(err, authorization.ErrorDenied) {
		t.Fatalf("GetPolicySnapshot() ordinary error = %v, want denied", err)
	}
}
