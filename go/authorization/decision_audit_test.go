package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestDecisionAuditRecordsDeniedAndHighRiskButNotOrdinaryAllow(t *testing.T) {
	definition := validDefinition()
	definition.Capabilities[0].Risk = authorization.RiskHigh
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	user := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "user"}
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	ctx := authorization.WithRequestMetadata(context.Background(), authorization.RequestMetadata{
		CorrelationID: "decision-request-7",
	})
	if _, err := module.Decide(ctx, authorization.DecisionRequest{
		Subject: user, Capability: "docs.document.publish", ScopeID: "docs",
	}); err != nil {
		t.Fatalf("Decide() denied error = %v", err)
	}
	adminDecision, err := module.Decide(ctx, authorization.DecisionRequest{
		Subject: admin, Capability: "docs.document.publish", ScopeID: "docs",
		Resource: authorization.ResourceFacts{
			Type: "document", ID: "doc-1", Revision: "v7",
		},
	})
	if err != nil {
		t.Fatalf("Decide() high-risk error = %v", err)
	}
	if !adminDecision.Allowed {
		t.Fatalf("Decide() high-risk = %#v, want allow", adminDecision)
	}

	page, err := module.SearchDecisionAudit(ctx, authorization.DecisionAuditQuery{
		CorrelationID: "decision-request-7",
	})
	if err != nil {
		t.Fatalf("SearchDecisionAudit() error = %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("SearchDecisionAudit().Total = %d, want denied + high-risk events", page.Total)
	}
	if page.Events[1].ResourceID != "doc-1" || page.Events[1].ResourceRevision != "v7" {
		t.Fatalf("high-risk event = %#v, want resource provenance", page.Events[1])
	}
	if page.Events[1].CorrelationID != "decision-request-7" {
		t.Fatalf("high-risk correlation = %q", page.Events[1].CorrelationID)
	}

	definition.Capabilities[0].Risk = authorization.RiskNormal
	ordinary, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() ordinary error = %v", err)
	}
	if _, err := ordinary.Decide(ctx, authorization.DecisionRequest{
		Subject: admin, Capability: "docs.document.publish", ScopeID: "docs",
	}); err != nil {
		t.Fatalf("Decide() ordinary allow error = %v", err)
	}
	if _, err := ordinary.Grant(ctx, authorization.GrantCommand{
		Actor: admin, Target: user, Role: "author", ScopeID: "docs",
		Source: authorization.GrantSourceDirect,
	}); err != nil {
		t.Fatalf("Grant() ordinary author error = %v", err)
	}
	if _, err := ordinary.Decide(ctx, authorization.DecisionRequest{
		Subject: user, Capability: "docs.document.publish", ScopeID: "docs",
	}); err != nil {
		t.Fatalf("Decide() ordinary author error = %v", err)
	}
	ordinaryPage, err := ordinary.SearchDecisionAudit(ctx, authorization.DecisionAuditQuery{})
	if err != nil {
		t.Fatalf("SearchDecisionAudit() ordinary error = %v", err)
	}
	if ordinaryPage.Total != 1 {
		t.Fatalf("ordinary protected allow audit total = %d, want protected allow recorded", ordinaryPage.Total)
	}
}
