package authorization_test

import (
	"context"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestRepositorySnapshotRestoresDecisionsAndInvitationSecrets(t *testing.T) {
	definition := validDefinition()
	definition.Roles[1].Assignment.Sources = append(
		definition.Roles[1].Assignment.Sources,
		authorization.GrantSourceInvitation,
	)
	catalog := authorization.MustCompile(definition)
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	author := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "author"}
	clock := func() time.Time { return time.Date(2026, 7, 23, 8, 0, 0, 0, time.UTC) }
	module, err := authorization.NewMemory(catalog, authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
		Clock: clock, TokenGenerator: func() (string, error) { return "secret-token", nil },
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	if _, err := module.Grant(context.Background(), authorization.GrantCommand{
		Actor: admin, Target: author, Role: "author", ScopeID: "docs",
		Source: authorization.GrantSourceDirect,
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	issue, err := module.Invite(context.Background(), authorization.InviteCommand{
		Actor: admin, Subject: authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "invitee"},
		Role: "author", ScopeID: "docs", ExpiresAt: clock().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Invite() error = %v", err)
	}

	restored, err := authorization.NewMemory(catalog, authorization.MemoryOptions{
		RootScopeID: "docs",
		ProtectedSubjects: []authorization.SubjectRef{
			{Kind: authorization.SubjectUser, ID: "placeholder"},
		},
		Clock: clock,
	})
	if err != nil {
		t.Fatalf("NewMemory() restore target error = %v", err)
	}
	if err := restored.RestoreRepositorySnapshot(module.RepositorySnapshot()); err != nil {
		t.Fatalf("RestoreRepositorySnapshot() error = %v", err)
	}
	decision, err := restored.Decide(context.Background(), authorization.DecisionRequest{
		Subject: author, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() restored error = %v", err)
	}
	if !decision.Allowed || len(decision.Sources) != 1 {
		t.Fatalf("Decide() restored = %#v, want role allow", decision)
	}
	accepted, err := restored.AcceptInvitation(context.Background(), authorization.AcceptInvitationCommand{
		Actor: authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "invitee"},
		Token: issue.Token,
	})
	if err != nil {
		t.Fatalf("AcceptInvitation() restored token error = %v", err)
	}
	if accepted.GrantID == "" {
		t.Fatalf("AcceptInvitation() = %#v, want grant", accepted)
	}
}
