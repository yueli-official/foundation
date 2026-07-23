package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestAuditReaderReturnsAppendOnlyGrantLifecycle(t *testing.T) {
	ctx := authorization.WithRequestMetadata(context.Background(), authorization.RequestMetadata{
		CorrelationID: "request-42",
	})
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	author := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "author"}
	module, err := authorization.NewMemory(authorization.MustCompile(validDefinition()), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	grant, err := module.Grant(ctx, authorization.GrantCommand{
		Actor: admin, Target: author, Role: "author", ScopeID: "docs", Source: authorization.GrantSourceDirect,
	})
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	if _, err := module.Revoke(ctx, authorization.RevokeCommand{Actor: admin, GrantID: grant.ID}); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	page, err := module.SearchAudit(ctx, authorization.AuditQuery{
		Subject: author, CorrelationID: "request-42",
	})
	if err != nil {
		t.Fatalf("SearchAudit() error = %v", err)
	}
	if page.Total != 2 || len(page.Events) != 2 {
		t.Fatalf("SearchAudit() = %#v, want two events", page)
	}
	if page.Events[0].Action != authorization.AuditGrantCreated ||
		page.Events[1].Action != authorization.AuditGrantRevoked {
		t.Fatalf("audit actions = %q, %q", page.Events[0].Action, page.Events[1].Action)
	}
	for _, event := range page.Events {
		if event.Actor != admin || event.Subject != author || event.Role != "author" ||
			event.ScopeID != "docs" || event.PolicyRevision != 1 {
			t.Fatalf("audit event = %#v, missing authorization provenance", event)
		}
		if event.CorrelationID != "request-42" {
			t.Fatalf("audit correlation = %q, want request-42", event.CorrelationID)
		}
	}
}
