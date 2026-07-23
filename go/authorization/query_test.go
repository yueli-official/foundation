package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestQueryPlannerReturnsTypedOwnerConstraintForAuthor(t *testing.T) {
	definition := validDefinition()
	definition.Capabilities[0].QueryableRelation = "owner"
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	author := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "author"}
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	if _, err := module.Grant(context.Background(), authorization.GrantCommand{
		Actor: admin, Target: author, Role: "author", ScopeID: "docs", Source: authorization.GrantSourceDirect,
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}

	plan, err := module.Plan(context.Background(), authorization.QueryRequest{
		Subject: author, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Kind != authorization.QueryRelation || plan.Relation != "owner" || plan.Subject != author {
		t.Fatalf("Plan() = %#v, want owner relation", plan)
	}

	adminPlan, err := module.Plan(context.Background(), authorization.QueryRequest{
		Subject: admin, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Plan() administrator error = %v", err)
	}
	if adminPlan.Kind != authorization.QueryAll {
		t.Fatalf("Plan() administrator = %#v, want all", adminPlan)
	}

	none, err := module.Plan(context.Background(), authorization.QueryRequest{
		Subject:    authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "reader"},
		Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Plan() reader error = %v", err)
	}
	if none.Kind != authorization.QueryNone {
		t.Fatalf("Plan() reader = %#v, want none", none)
	}
}
