package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestGroupGrantAppliesToMembersWithoutAllowingNestedGroups(t *testing.T) {
	ctx := context.Background()
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	member := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "writer"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	group, err := module.CreateGroup(ctx, authorization.CreateGroupCommand{
		Actor: admin, ID: "editorial", ScopeID: "docs", DisplayName: "编辑组",
	})
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}
	if _, err := module.AddGroupMember(ctx, authorization.AddGroupMemberCommand{
		Actor: admin, GroupID: group.ID, Member: member,
	}); err != nil {
		t.Fatalf("AddGroupMember() error = %v", err)
	}
	grant, err := module.Grant(ctx, authorization.GrantCommand{
		Actor:  admin,
		Target: authorization.SubjectRef{Kind: authorization.SubjectGroup, ID: string(group.ID)},
		Role:   "author", ScopeID: "docs", Source: authorization.GrantSourceGroup,
	})
	if err != nil {
		t.Fatalf("Grant() group error = %v", err)
	}

	decision, err := module.Decide(ctx, authorization.DecisionRequest{
		Subject: member, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if !decision.Allowed || len(decision.Sources) != 1 ||
		decision.Sources[0].GrantID != grant.ID || decision.Sources[0].GroupID != group.ID {
		t.Fatalf("Decide() = %#v, want group grant provenance", decision)
	}

	_, err = module.AddGroupMember(ctx, authorization.AddGroupMemberCommand{
		Actor: admin, GroupID: group.ID,
		Member: authorization.SubjectRef{Kind: authorization.SubjectGroup, ID: "another-group"},
	})
	if !authorization.Is(err, authorization.ErrorInvariant) {
		t.Fatalf("AddGroupMember() nested error = %v, want invariant violation", err)
	}
}
