package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestAutomaticRuleCreatesExplicitIdempotentGrant(t *testing.T) {
	definition := validDefinition()
	definition.Roles[1].Assignment.Sources = append(
		definition.Roles[1].Assignment.Sources,
		authorization.GrantSourceAutomatic,
	)
	definition.Automatic = []authorization.AutomaticRuleDefinition{
		{
			Key: "docs.registration_author", Trigger: "identity.user.registered",
			Predicate: "identity.email_verified", Role: "author", Enabled: true,
		},
	}
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	subject := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "new-user"}
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
		Predicates: map[authorization.PredicateKey]authorization.PredicateEvaluator{
			"identity.email_verified": authorization.PredicateFunc(func(_ context.Context, input authorization.PredicateInput) bool {
				verified, _ := input.Facts["email_verified"].(bool)
				return verified
			}),
		},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	event := authorization.AutomaticEvent{
		ID: "event-1", Trigger: "identity.user.registered", Subject: subject,
		Facts: map[authorization.FactKey]any{"email_verified": true},
	}
	first, err := module.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}
	second, err := module.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleEvent() replay error = %v", err)
	}
	if len(first.Grants) != 1 || len(second.Grants) != 1 || first.Grants[0].ID != second.Grants[0].ID {
		t.Fatalf("HandleEvent() results = %#v / %#v, want same one grant", first, second)
	}
	if first.Grants[0].Source != authorization.GrantSourceAutomatic {
		t.Fatalf("automatic grant source = %q", first.Grants[0].Source)
	}

	decision, err := module.Decide(context.Background(), authorization.DecisionRequest{
		Subject: subject, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("Decide() = %#v, want automatic grant allow", decision)
	}
}

func TestAutomaticRulePolicySwitchAffectsFutureEventsWithoutRevokingExistingGrant(t *testing.T) {
	definition := validDefinition()
	definition.Roles[1].Assignment.Sources = append(
		definition.Roles[1].Assignment.Sources,
		authorization.GrantSourceAutomatic,
	)
	definition.Automatic = []authorization.AutomaticRuleDefinition{
		{
			Key: "docs.registration_author", Trigger: "identity.user.registered",
			Predicate: "identity.email_verified", Role: "author", Enabled: true,
		},
	}
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
		Predicates: map[authorization.PredicateKey]authorization.PredicateEvaluator{
			"identity.email_verified": authorization.PredicateFunc(func(context.Context, authorization.PredicateInput) bool {
				return true
			}),
		},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	ctx := context.Background()
	existing := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "existing"}
	if _, err := module.HandleEvent(ctx, authorization.AutomaticEvent{
		ID: "existing-event", Trigger: "identity.user.registered", Subject: existing,
	}); err != nil {
		t.Fatalf("HandleEvent() existing error = %v", err)
	}
	draft, err := module.CreatePolicyDraft(ctx, authorization.CreatePolicyDraftCommand{
		Actor: admin, ScopeID: "docs", ExpectedActiveRevision: 1,
	})
	if err != nil {
		t.Fatalf("CreatePolicyDraft() error = %v", err)
	}
	if _, err := module.SetAutomaticRuleEnabled(ctx, authorization.SetAutomaticRuleEnabledCommand{
		Actor: admin, Revision: draft.Number, Rule: "docs.registration_author", Enabled: false,
	}); err != nil {
		t.Fatalf("SetAutomaticRuleEnabled() error = %v", err)
	}
	if _, err := module.ActivatePolicy(ctx, authorization.ActivatePolicyCommand{
		Actor: admin, Revision: draft.Number, ExpectedActiveRevision: 1,
	}); err != nil {
		t.Fatalf("ActivatePolicy() error = %v", err)
	}
	future := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "future"}
	result, err := module.HandleEvent(ctx, authorization.AutomaticEvent{
		ID: "future-event", Trigger: "identity.user.registered", Subject: future,
	})
	if err != nil {
		t.Fatalf("HandleEvent() disabled error = %v", err)
	}
	if result.Created != 0 {
		t.Fatalf("HandleEvent() disabled = %#v, want no future grant", result)
	}
	decision, err := module.Decide(ctx, authorization.DecisionRequest{
		Subject: existing, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() existing error = %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("Decide() existing = %#v, want existing automatic grant retained", decision)
	}
}

func TestAutomaticReconcilePreviewDoesNotCreateGrant(t *testing.T) {
	definition := validDefinition()
	definition.Roles[1].Assignment.Sources = append(
		definition.Roles[1].Assignment.Sources,
		authorization.GrantSourceAutomatic,
	)
	definition.Automatic = []authorization.AutomaticRuleDefinition{
		{
			Key: "docs.registration_author", Trigger: "identity.user.registered",
			Predicate: "identity.email_verified", Role: "author", Enabled: true,
		},
	}
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	subject := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "preview-user"}
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
		Predicates: map[authorization.PredicateKey]authorization.PredicateEvaluator{
			"identity.email_verified": authorization.PredicateFunc(func(context.Context, authorization.PredicateInput) bool {
				return true
			}),
		},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	preview, err := module.PreviewReconcileSubject(context.Background(), authorization.ReconcileSubjectCommand{
		Subject: subject,
	})
	if err != nil {
		t.Fatalf("PreviewReconcileSubject() error = %v", err)
	}
	if preview.Created != 1 || len(preview.Grants) != 1 || preview.Grants[0].ID != "" {
		t.Fatalf("PreviewReconcileSubject() = %#v, want one non-persisted proposed grant", preview)
	}
	decision, err := module.Decide(context.Background(), authorization.DecisionRequest{
		Subject: subject, Capability: "docs.document.publish", ScopeID: "docs",
	})
	if err != nil {
		t.Fatalf("Decide() after preview error = %v", err)
	}
	if decision.Allowed {
		t.Fatalf("Decide() after preview = %#v, want no persisted grant", decision)
	}
}
