package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestApprovedApplicationCreatesGrantAndTerminalStateCannotBeRewritten(t *testing.T) {
	ctx := context.Background()
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	applicant := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "applicant"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}

	application, err := module.Apply(ctx, authorization.ApplyCommand{
		Actor: applicant, Role: "author", ScopeID: "docs", Reason: "I write the handbook",
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if application.State != authorization.ApplicationPending {
		t.Fatalf("Apply() state = %q, want pending", application.State)
	}

	approved, err := module.ReviewApplication(ctx, authorization.ReviewApplicationCommand{
		Actor: admin, ApplicationID: application.ID, Decision: authorization.ReviewApprove,
	})
	if err != nil {
		t.Fatalf("ReviewApplication() error = %v", err)
	}
	if approved.State != authorization.ApplicationApproved || approved.GrantID == "" {
		t.Fatalf("ReviewApplication() = %#v, want approved with grant", approved)
	}

	_, err = module.ReviewApplication(ctx, authorization.ReviewApplicationCommand{
		Actor: admin, ApplicationID: application.ID, Decision: authorization.ReviewReject,
	})
	if !authorization.Is(err, authorization.ErrorConflict) {
		t.Fatalf("second ReviewApplication() error = %v, want conflict", err)
	}

	decision, err := module.Decide(ctx, authorization.DecisionRequest{
		Subject: applicant, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if !decision.Allowed || len(decision.Sources) != 1 || decision.Sources[0].GrantID != approved.GrantID {
		t.Fatalf("Decide() = %#v, want application grant", decision)
	}
}

func TestProtectedRoleCannotBeRequested(t *testing.T) {
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	_, err = module.Apply(context.Background(), authorization.ApplyCommand{
		Actor: authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "applicant"},
		Role:  "administrator", ScopeID: "docs",
	})
	if !authorization.Is(err, authorization.ErrorInvalidInput) {
		t.Fatalf("Apply() protected role error = %v, want invalid input", err)
	}
}

func TestApplicationIdempotencyKeyReplaysOnlyTheSameCommand(t *testing.T) {
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	applicant := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "applicant"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	command := authorization.ApplyCommand{
		Actor: applicant, Role: "author", ScopeID: "docs", Reason: "write",
		IdempotencyKey: "application-request-1",
	}
	first, err := module.Apply(context.Background(), command)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	second, err := module.Apply(context.Background(), command)
	if err != nil || second.ID != first.ID {
		t.Fatalf("Apply() replay = %#v, %v; want %#v", second, err, first)
	}
	command.Reason = "different"
	if _, err := module.Apply(context.Background(), command); !authorization.Is(err, authorization.ErrorConflict) {
		t.Fatalf("Apply() reused key error = %v, want conflict", err)
	}
}
