package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/yueli-official/foundation/go/authorization"
	"github.com/yueli-official/foundation/go/authorization/authorizationtest"
	authorizationpostgres "github.com/yueli-official/foundation/go/authorization/postgres"
)

func TestPostgresAdapterConformance(t *testing.T) {
	database := openPostgresTestDatabase(t)
	var instanceCounter atomic.Uint64
	authorizationtest.Run(t, func(ctx context.Context, setup authorizationtest.Setup) (authorizationtest.Adapter, func(), error) {
		catalog, err := authorization.Compile(setup.Definition)
		if err != nil {
			return nil, nil, err
		}
		adapter, err := authorizationpostgres.New(ctx, catalog, authorizationpostgres.Options{
			DB: database, InstanceKey: fmt.Sprintf("conformance-%d", instanceCounter.Add(1)),
			Memory: authorization.MemoryOptions{
				RootScopeID:       setup.RootScopeID,
				ProtectedSubjects: setup.ProtectedSubjects,
			},
		})
		return adapter, nil, err
	})
}

func TestPostgresSchemaDownRemovesAllAuthorizationTables(t *testing.T) {
	database := openPostgresTestDatabase(t)
	migration, err := authorizationpostgres.Schema(authorizationpostgres.CurrentSchemaVersion)
	if err != nil {
		t.Fatalf("Schema() error = %v", err)
	}
	if _, err := database.Exec(migration.DownSQL); err != nil {
		t.Fatalf("schema down error = %v", err)
	}
	var relation sql.NullString
	if err := database.QueryRow(`SELECT to_regclass('authorization_instances')`).Scan(&relation); err != nil {
		t.Fatalf("to_regclass() error = %v", err)
	}
	if relation.Valid {
		t.Fatalf("authorization_instances still exists as %q", relation.String)
	}
}

func TestPostgresAdapterRestartRestoresDomainTruth(t *testing.T) {
	database := openPostgresTestDatabase(t)
	definition := postgresTestDefinition()
	catalog := authorization.MustCompile(definition)
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	author := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "author"}
	ctx := context.Background()
	first, err := authorizationpostgres.New(ctx, catalog, authorizationpostgres.Options{
		DB: database, InstanceKey: "restart",
		Memory: authorization.MemoryOptions{
			RootScopeID: "site", ProtectedSubjects: []authorization.SubjectRef{admin},
		},
	})
	if err != nil {
		t.Fatalf("New() first error = %v", err)
	}
	grant, err := first.Grant(ctx, authorization.GrantCommand{
		Actor: admin, Target: author, Role: "author", ScopeID: "site",
		Source: authorization.GrantSourceDirect,
	})
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}

	restarted, err := authorizationpostgres.New(ctx, catalog, authorizationpostgres.Options{
		DB: database, InstanceKey: "restart",
		Memory: authorization.MemoryOptions{RootScopeID: "ignored-on-restart"},
	})
	if err != nil {
		t.Fatalf("New() restart error = %v", err)
	}
	decision, err := restarted.Decide(ctx, authorization.DecisionRequest{
		Subject: author, Capability: "content.publish", ScopeID: "site",
	})
	if err != nil {
		t.Fatalf("Decide() restart error = %v", err)
	}
	if !decision.Allowed || len(decision.Sources) != 1 || decision.Sources[0].GrantID != grant.ID {
		t.Fatalf("Decide() restart = %#v, want original grant provenance", decision)
	}
	if _, err := restarted.Decide(ctx, authorization.DecisionRequest{
		Subject: admin, Capability: "content.publish", ScopeID: "site",
		Resource: authorization.ResourceFacts{Type: "document", ID: "admin-doc", Revision: "r1"},
	}); err != nil {
		t.Fatalf("Decide() protected audit error = %v", err)
	}
	draft, err := restarted.CreatePolicyDraft(ctx, authorization.CreatePolicyDraftCommand{
		Actor: admin, ScopeID: "site", ExpectedActiveRevision: 1,
	})
	if err != nil {
		t.Fatalf("CreatePolicyDraft() error = %v", err)
	}
	if _, err := restarted.SetRoleCapabilities(ctx, authorization.SetRoleCapabilitiesCommand{
		Actor: admin, Revision: draft.Number, Role: "author",
	}); err != nil {
		t.Fatalf("SetRoleCapabilities() removal error = %v", err)
	}
	if _, err := restarted.ActivatePolicy(ctx, authorization.ActivatePolicyCommand{
		Actor: admin, Revision: draft.Number, ExpectedActiveRevision: 1,
	}); err != nil {
		t.Fatalf("ActivatePolicy() removal error = %v", err)
	}
	restartedAgain, err := authorizationpostgres.New(ctx, catalog, authorizationpostgres.Options{
		DB: database, InstanceKey: "restart",
		Memory: authorization.MemoryOptions{RootScopeID: "ignored-again"},
	})
	if err != nil {
		t.Fatalf("New() second restart error = %v", err)
	}
	decisionAudit, err := restartedAgain.SearchDecisionAudit(ctx, authorization.DecisionAuditQuery{})
	if err != nil {
		t.Fatalf("SearchDecisionAudit() restart error = %v", err)
	}
	if decisionAudit.Total != 1 || decisionAudit.Events[0].ResourceID != "admin-doc" {
		t.Fatalf("SearchDecisionAudit() restart = %#v, want protected decision provenance", decisionAudit)
	}
	denied, err := restartedAgain.Decide(ctx, authorization.DecisionRequest{
		Subject: author, Capability: "content.publish", ScopeID: "site",
	})
	if err != nil {
		t.Fatalf("Decide() removed binding error = %v", err)
	}
	if denied.Allowed {
		t.Fatalf("Decide() removed binding = %#v, want deny after restart", denied)
	}
	if _, err := database.Exec(`
		DELETE FROM authorization_projection_rules WHERE instance_key = 'restart'
	`); err != nil {
		t.Fatalf("delete projection error = %v", err)
	}
	if err := restartedAgain.RebuildProjection(ctx); err != nil {
		t.Fatalf("RebuildProjection() error = %v", err)
	}
	var projectionRules int
	if err := database.QueryRow(`
		SELECT count(*) FROM authorization_projection_rules WHERE instance_key = 'restart'
	`).Scan(&projectionRules); err != nil {
		t.Fatalf("count projection error = %v", err)
	}
	if projectionRules == 0 {
		t.Fatal("RebuildProjection() left the derived projection empty")
	}
}

func TestPostgresAdapterSerializesCompetingApplicationApproval(t *testing.T) {
	database := openPostgresTestDatabase(t)
	definition := postgresTestDefinition()
	catalog := authorization.MustCompile(definition)
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	applicant := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "applicant"}
	ctx := context.Background()
	adapter, err := authorizationpostgres.New(ctx, catalog, authorizationpostgres.Options{
		DB: database, InstanceKey: "approval-race",
		Memory: authorization.MemoryOptions{
			RootScopeID: "site", ProtectedSubjects: []authorization.SubjectRef{admin},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	application, err := adapter.Apply(ctx, authorization.ApplyCommand{
		Actor: applicant, Role: "author", ScopeID: "site",
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, reviewErr := adapter.ReviewApplication(ctx, authorization.ReviewApplicationCommand{
				Actor: admin, ApplicationID: application.ID, Decision: authorization.ReviewApprove,
			})
			results <- reviewErr
		}()
	}
	successes := 0
	conflicts := 0
	for range 2 {
		result := <-results
		if result == nil {
			successes++
		} else if authorization.Is(result, authorization.ErrorConflict) {
			conflicts++
		} else {
			t.Fatalf("ReviewApplication() race error = %v", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("ReviewApplication() race successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}
}

func TestPostgresAdapterPreservesOneAdministratorUnderCompetingRevokes(t *testing.T) {
	database := openPostgresTestDatabase(t)
	catalog := authorization.MustCompile(postgresTestDefinition())
	firstAdmin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin-1"}
	secondAdmin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin-2"}
	ctx := context.Background()
	adapter, err := authorizationpostgres.New(ctx, catalog, authorizationpostgres.Options{
		DB: database, InstanceKey: "admin-race",
		Memory: authorization.MemoryOptions{
			RootScopeID: "site",
			ProtectedSubjects: []authorization.SubjectRef{
				firstAdmin,
				secondAdmin,
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	firstAccess, err := adapter.EffectiveAccess(ctx, authorization.EffectiveAccessQuery{
		Subject: firstAdmin, ScopeID: "site",
	})
	if err != nil {
		t.Fatalf("EffectiveAccess(first) error = %v", err)
	}
	secondAccess, err := adapter.EffectiveAccess(ctx, authorization.EffectiveAccessQuery{
		Subject: secondAdmin, ScopeID: "site",
	})
	if err != nil {
		t.Fatalf("EffectiveAccess(second) error = %v", err)
	}
	results := make(chan error, 2)
	go func() {
		_, revokeErr := adapter.Revoke(ctx, authorization.RevokeCommand{
			Actor: firstAdmin, GrantID: secondAccess.Grants[0].ID,
		})
		results <- revokeErr
	}()
	go func() {
		_, revokeErr := adapter.Revoke(ctx, authorization.RevokeCommand{
			Actor: secondAdmin, GrantID: firstAccess.Grants[0].ID,
		})
		results <- revokeErr
	}()
	successes := 0
	rejections := 0
	for range 2 {
		result := <-results
		if result == nil {
			successes++
		} else if authorization.Is(result, authorization.ErrorDenied) ||
			authorization.Is(result, authorization.ErrorInvariant) {
			rejections++
		} else {
			t.Fatalf("Revoke() race error = %v", result)
		}
	}
	if successes != 1 || rejections != 1 {
		t.Fatalf("Revoke() race successes=%d rejections=%d, want 1/1", successes, rejections)
	}
}

func TestPostgresAdapterRestoresAutomaticRulePolicyAndInbox(t *testing.T) {
	database := openPostgresTestDatabase(t)
	definition := postgresTestDefinition()
	definition.Roles[1].Assignment.Sources = append(
		definition.Roles[1].Assignment.Sources,
		authorization.GrantSourceAutomatic,
	)
	definition.Automatic = []authorization.AutomaticRuleDefinition{
		{
			Key: "test.registration_author", Trigger: "identity.user.registered",
			Predicate: "identity.email_verified", Role: "author", Enabled: true,
		},
	}
	catalog := authorization.MustCompile(definition)
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	predicate := authorization.PredicateFunc(func(context.Context, authorization.PredicateInput) bool {
		return true
	})
	ctx := context.Background()
	options := authorizationpostgres.Options{
		DB: database, InstanceKey: "automatic-restart",
		Memory: authorization.MemoryOptions{
			RootScopeID: "site", ProtectedSubjects: []authorization.SubjectRef{admin},
			Predicates: map[authorization.PredicateKey]authorization.PredicateEvaluator{
				"identity.email_verified": predicate,
			},
		},
	}
	first, err := authorizationpostgres.New(ctx, catalog, options)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	existing := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "existing"}
	event := authorization.AutomaticEvent{
		ID: "registration-1", Trigger: "identity.user.registered", Subject: existing,
	}
	created, err := first.HandleEvent(ctx, event)
	if err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}
	if created.Created != 1 {
		t.Fatalf("HandleEvent() = %#v, want created grant", created)
	}
	draft, err := first.CreatePolicyDraft(ctx, authorization.CreatePolicyDraftCommand{
		Actor: admin, ScopeID: "site", ExpectedActiveRevision: 1,
	})
	if err != nil {
		t.Fatalf("CreatePolicyDraft() error = %v", err)
	}
	if _, err := first.SetAutomaticRuleEnabled(ctx, authorization.SetAutomaticRuleEnabledCommand{
		Actor: admin, Revision: draft.Number, Rule: "test.registration_author", Enabled: false,
	}); err != nil {
		t.Fatalf("SetAutomaticRuleEnabled() error = %v", err)
	}
	if _, err := first.ActivatePolicy(ctx, authorization.ActivatePolicyCommand{
		Actor: admin, Revision: draft.Number, ExpectedActiveRevision: 1,
	}); err != nil {
		t.Fatalf("ActivatePolicy() error = %v", err)
	}
	options.Memory.ProtectedSubjects = nil
	restarted, err := authorizationpostgres.New(ctx, catalog, options)
	if err != nil {
		t.Fatalf("New() restart error = %v", err)
	}
	replayed, err := restarted.HandleEvent(ctx, event)
	if err != nil {
		t.Fatalf("HandleEvent() replay error = %v", err)
	}
	if len(replayed.Grants) != 1 || replayed.Grants[0].ID != created.Grants[0].ID {
		t.Fatalf("HandleEvent() replay = %#v, want persisted inbox result", replayed)
	}
	future, err := restarted.HandleEvent(ctx, authorization.AutomaticEvent{
		ID: "registration-2", Trigger: "identity.user.registered",
		Subject: authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "future"},
	})
	if err != nil {
		t.Fatalf("HandleEvent() disabled error = %v", err)
	}
	if future.Created != 0 {
		t.Fatalf("HandleEvent() disabled = %#v, want no new grant", future)
	}
}

func TestPostgresAdapterFailsClosedWhenCommitStoreIsUnavailable(t *testing.T) {
	database := openPostgresTestDatabase(t)
	catalog := authorization.MustCompile(postgresTestDefinition())
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	adapter, err := authorizationpostgres.New(context.Background(), catalog, authorizationpostgres.Options{
		DB: database, InstanceKey: "fail-closed",
		Memory: authorization.MemoryOptions{
			RootScopeID: "site", ProtectedSubjects: []authorization.SubjectRef{admin},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	_, err = adapter.Decide(context.Background(), authorization.DecisionRequest{
		Subject: admin, Capability: "content.publish", ScopeID: "site",
	})
	if !authorization.Is(err, authorization.ErrorUnavailable) {
		t.Fatalf("Decide() unavailable store error = %v, want unavailable", err)
	}
}

func TestOfflineRecoveryRequiresDryRunAndExactConfirmation(t *testing.T) {
	database := openPostgresTestDatabase(t)
	catalog := authorization.MustCompile(postgresTestDefinition())
	lostAdmin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "lost-admin"}
	recoveredAdmin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "recovered-admin"}
	ctx := context.Background()
	if _, err := authorizationpostgres.New(ctx, catalog, authorizationpostgres.Options{
		DB: database, InstanceKey: "recovery",
		Memory: authorization.MemoryOptions{
			RootScopeID: "site", ProtectedSubjects: []authorization.SubjectRef{lostAdmin},
		},
	}); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := database.Exec(`
		UPDATE authorization_grants
		SET revoked_at = now()
		WHERE instance_key = 'recovery'
	`); err != nil {
		t.Fatalf("simulate lost administrator error = %v", err)
	}
	preview, err := authorizationpostgres.RecoverProtectedAdministrator(ctx, authorizationpostgres.RecoveryCommand{
		DB: database, InstanceKey: "recovery", Target: recoveredAdmin, DryRun: true,
	})
	if err != nil {
		t.Fatalf("RecoverProtectedAdministrator() dry-run error = %v", err)
	}
	if !preview.WouldCreate || preview.GrantID != "" {
		t.Fatalf("dry-run = %#v, want non-mutating recovery plan", preview)
	}
	_, err = authorizationpostgres.RecoverProtectedAdministrator(ctx, authorizationpostgres.RecoveryCommand{
		DB: database, InstanceKey: "recovery", Target: recoveredAdmin, Confirmation: "wrong",
	})
	if !authorization.Is(err, authorization.ErrorInvalidInput) {
		t.Fatalf("RecoverProtectedAdministrator() confirmation error = %v, want invalid input", err)
	}
	recovered, err := authorizationpostgres.RecoverProtectedAdministrator(ctx, authorizationpostgres.RecoveryCommand{
		DB: database, InstanceKey: "recovery", Target: recoveredAdmin,
		Confirmation: "recovery:user:recovered-admin",
	})
	if err != nil {
		t.Fatalf("RecoverProtectedAdministrator() error = %v", err)
	}
	if recovered.GrantID == "" || !recovered.RequiresProjectionRebuild {
		t.Fatalf("recovery = %#v, want created grant and rebuild signal", recovered)
	}
	restarted, err := authorizationpostgres.New(ctx, catalog, authorizationpostgres.Options{
		DB: database, InstanceKey: "recovery",
		Memory: authorization.MemoryOptions{RootScopeID: "ignored"},
	})
	if err != nil {
		t.Fatalf("New() after recovery error = %v", err)
	}
	decision, err := restarted.Decide(ctx, authorization.DecisionRequest{
		Subject: recoveredAdmin, Capability: "content.publish", ScopeID: "site",
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("Decide() recovered = %#v, %v, want allow", decision, err)
	}
}

func TestPostgresApplicationIdempotencyAndAuditCorrelationSurviveRestart(t *testing.T) {
	database := openPostgresTestDatabase(t)
	catalog := authorization.MustCompile(postgresTestDefinition())
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	applicant := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "applicant"}
	ctx := authorization.WithRequestMetadata(context.Background(), authorization.RequestMetadata{
		CorrelationID: "integration-request-1",
	})
	module, err := authorizationpostgres.New(ctx, catalog, authorizationpostgres.Options{
		DB: database, InstanceKey: "idempotency-correlation",
		Memory: authorization.MemoryOptions{
			RootScopeID: "site", ProtectedSubjects: []authorization.SubjectRef{admin},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	command := authorization.ApplyCommand{
		Actor: applicant, Role: "author", ScopeID: "site", Reason: "write",
		IdempotencyKey: "application-1",
	}
	first, err := module.Apply(ctx, command)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	restarted, err := authorizationpostgres.New(ctx, catalog, authorizationpostgres.Options{
		DB: database, InstanceKey: "idempotency-correlation",
		Memory: authorization.MemoryOptions{RootScopeID: "ignored"},
	})
	if err != nil {
		t.Fatalf("New() restart error = %v", err)
	}
	replayed, err := restarted.Apply(ctx, command)
	if err != nil || replayed.ID != first.ID {
		t.Fatalf("Apply() restart replay = %#v, %v; want %#v", replayed, err, first)
	}
	audit, err := restarted.SearchAudit(ctx, authorization.AuditQuery{
		CorrelationID: "integration-request-1",
	})
	if err != nil {
		t.Fatalf("SearchAudit() error = %v", err)
	}
	created := 0
	for _, event := range audit.Events {
		if event.Action == authorization.AuditApplicationCreated {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("application-created events = %d, want one idempotent event", created)
	}
}

func openPostgresTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("AUTHORIZATION_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("AUTHORIZATION_POSTGRES_DSN is not configured")
	}
	database, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	database.SetMaxOpenConns(1)
	schemaName := fmt.Sprintf("authorization_test_%d", time.Now().UnixNano())
	if _, err := database.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schemaName)); err != nil {
		_ = database.Close()
		t.Fatalf("CREATE SCHEMA error = %v", err)
	}
	if _, err := database.Exec("SET search_path TO " + pq.QuoteIdentifier(schemaName)); err != nil {
		_ = database.Close()
		t.Fatalf("SET search_path error = %v", err)
	}
	migration, err := authorizationpostgres.Schema(authorizationpostgres.CurrentSchemaVersion)
	if err != nil {
		t.Fatalf("Schema() error = %v", err)
	}
	if _, err := database.Exec(migration.UpSQL); err != nil {
		t.Fatalf("schema up error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(migration.DownSQL)
		_, _ = database.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schemaName))
		_ = database.Close()
	})
	return database
}

func postgresTestDefinition() authorization.Definition {
	return authorization.Definition{
		Consumer: "postgres-test",
		Version:  1,
		Capabilities: []authorization.CapabilityDefinition{
			{Key: "content.publish", Version: 1, Binding: authorization.BindingNormal},
		},
		Scopes: authorization.ScopeSchema{Types: []authorization.ScopeTypeDefinition{
			{Key: "site", Root: true},
		}},
		AccessLayers: []authorization.AccessLayerDefinition{
			{Key: authorization.AccessLayerVisitor},
			{Key: authorization.AccessLayerAuthenticated},
		},
		Roles: []authorization.RoleDefinition{
			{
				Key: "administrator", DisplayName: "Administrator", Protected: true,
				Capabilities: []authorization.CapabilityKey{
					authorization.CapabilityManage,
					"content.publish",
				},
			},
			{
				Key: "author", DisplayName: "Author",
				Capabilities: []authorization.CapabilityKey{"content.publish"},
				Assignment: authorization.AssignmentPolicy{
					Sources: []authorization.GrantSource{
						authorization.GrantSourceApplication,
						authorization.GrantSourceDirect,
					},
				},
			},
		},
	}
}
