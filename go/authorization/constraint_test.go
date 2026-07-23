package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestSourceConstraintRejectsAuthorButNotIndependentAdministratorSource(t *testing.T) {
	definition := validDefinition()
	definition.Constraints = []authorization.ConstraintDefinition{
		{
			Key: "docs.author_owns_document", Version: 1, Mode: authorization.ConstraintSource,
			Capabilities: []authorization.CapabilityKey{"docs.document.publish"},
			Roles:        []authorization.RoleKey{"author"},
		},
	}
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	author := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "author"}
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
		Constraints: map[authorization.ConstraintKey]authorization.ConstraintEvaluator{
			"docs.author_owns_document": authorization.ConstraintFunc(func(_ context.Context, input authorization.ConstraintInput) authorization.ConstraintResult {
				for _, owner := range input.Resource.Relations["owner"] {
					if owner == input.Subject {
						return authorization.ConstraintResult{}
					}
				}
				return authorization.ConstraintResult{Denied: true}
			}),
		},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	if _, err := module.Grant(context.Background(), authorization.GrantCommand{
		Actor: admin, Target: author, Role: "author", ScopeID: "docs", Source: authorization.GrantSourceDirect,
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}

	otherOwner := authorization.ResourceFacts{
		ScopeID: "docs",
		Relations: map[authorization.RelationKind][]authorization.SubjectRef{
			"owner": {{Kind: authorization.SubjectUser, ID: "someone-else"}},
		},
	}
	denied, err := module.Decide(context.Background(), authorization.DecisionRequest{
		Subject: author, Capability: "docs.document.publish", ScopeID: "docs", Resource: otherOwner,
	})
	if err != nil {
		t.Fatalf("Decide() author error = %v", err)
	}
	if denied.Allowed || denied.Reason != authorization.ReasonConstraint || denied.Constraint != "docs.author_owns_document" {
		t.Fatalf("Decide() author = %#v, want constraint deny", denied)
	}

	adminDecision, err := module.Decide(context.Background(), authorization.DecisionRequest{
		Subject: admin, Capability: "docs.document.publish", ScopeID: "docs", Resource: otherOwner,
	})
	if err != nil {
		t.Fatalf("Decide() administrator error = %v", err)
	}
	if !adminDecision.Allowed {
		t.Fatalf("Decide() administrator = %#v, want independent protected-role allow", adminDecision)
	}
}

func TestGlobalConstraintRejectsProtectedAdministrator(t *testing.T) {
	definition := validDefinition()
	definition.Constraints = []authorization.ConstraintDefinition{
		{
			Key: "docs.resource_is_active", Version: 1, Mode: authorization.ConstraintGlobal,
			Capabilities: []authorization.CapabilityKey{"docs.document.publish"},
		},
	}
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
		Constraints: map[authorization.ConstraintKey]authorization.ConstraintEvaluator{
			"docs.resource_is_active": authorization.ConstraintFunc(func(context.Context, authorization.ConstraintInput) authorization.ConstraintResult {
				return authorization.ConstraintResult{Denied: true}
			}),
		},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}

	decision, err := module.Decide(context.Background(), authorization.DecisionRequest{
		Subject: admin, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Allowed || decision.Reason != authorization.ReasonConstraint || decision.Constraint != "docs.resource_is_active" {
		t.Fatalf("Decide() = %#v, want global constraint deny", decision)
	}
}
