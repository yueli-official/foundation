package authorization_test

import (
	"context"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestEmailInvitationCreatesGrantOnlyAfterVerifiedAcceptance(t *testing.T) {
	definition := validDefinition()
	definition.Roles[1].Assignment.Sources = append(
		definition.Roles[1].Assignment.Sources,
		authorization.GrantSourceInvitation,
	)
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	invitee := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "invitee"}
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
		Clock: func() time.Time { return now },
		TokenGenerator: func() (string, error) {
			return "one-time-secret", nil
		},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}

	issued, err := module.Invite(context.Background(), authorization.InviteCommand{
		Actor: admin, Email: " Writer@Example.COM ", Role: "author", ScopeID: "docs",
		ExpiresAt: now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Invite() error = %v", err)
	}
	if issued.Invitation.State != authorization.InvitationPending || issued.Token != "one-time-secret" {
		t.Fatalf("Invite() = %#v, want pending invitation and one-time token", issued)
	}
	before, err := module.Decide(context.Background(), authorization.DecisionRequest{
		Subject: invitee, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() before acceptance error = %v", err)
	}
	if before.Allowed {
		t.Fatal("email invitation granted access before acceptance")
	}

	accepted, err := module.AcceptInvitation(context.Background(), authorization.AcceptInvitationCommand{
		Actor: invitee, VerifiedEmail: "writer@example.com", Token: issued.Token,
	})
	if err != nil {
		t.Fatalf("AcceptInvitation() error = %v", err)
	}
	if accepted.State != authorization.InvitationAccepted || accepted.GrantID == "" {
		t.Fatalf("AcceptInvitation() = %#v, want accepted with grant", accepted)
	}
	after, err := module.Decide(context.Background(), authorization.DecisionRequest{
		Subject: invitee, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() after acceptance error = %v", err)
	}
	if !after.Allowed {
		t.Fatalf("Decide() after acceptance = %#v, want allow", after)
	}

	_, err = module.AcceptInvitation(context.Background(), authorization.AcceptInvitationCommand{
		Actor: invitee, VerifiedEmail: "writer@example.com", Token: issued.Token,
	})
	if !authorization.Is(err, authorization.ErrorConflict) {
		t.Fatalf("second AcceptInvitation() error = %v, want conflict", err)
	}
}
