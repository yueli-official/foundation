package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/yueli-official/foundation/go/authorization"
)

type Options struct {
	DB          *sql.DB
	InstanceKey string
	Memory      authorization.MemoryOptions
}

// Adapter persists domain truth in the consumer's PostgreSQL database while
// executing the same domain kernel as the reference Memory Adapter.
type Adapter struct {
	mu      sync.RWMutex
	catalog *authorization.Catalog
	options authorization.MemoryOptions
	store   stateStore
	memory  *authorization.Memory
	created bool
}

// InstanceWasCreated reports whether New bootstrapped a previously absent
// authorization instance. Consumers use this narrow lifecycle signal for
// one-time resource-scope backfills; it does not expose repository state.
func (adapter *Adapter) InstanceWasCreated() bool {
	return adapter != nil && adapter.created
}

// RebuildProjection recreates both the in-process Casbin snapshot and the
// inspectable PostgreSQL projection solely from domain truth.
func (adapter *Adapter) RebuildProjection(ctx context.Context) error {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if err := adapter.memory.RebuildCasbinProjection(); err != nil {
		return err
	}
	if err := adapter.store.rebuildProjection(ctx, adapter.memory.RepositorySnapshot()); err != nil {
		return unavailable("rebuild execution projection", err)
	}
	return nil
}

var (
	_ authorization.Authorizer            = (*Adapter)(nil)
	_ authorization.QueryPlanner          = (*Adapter)(nil)
	_ authorization.AccessReader          = (*Adapter)(nil)
	_ authorization.ScopeManager          = (*Adapter)(nil)
	_ authorization.ResourceScopeRegistry = (*Adapter)(nil)
	_ authorization.ScopeReader           = (*Adapter)(nil)
	_ authorization.RoleManager           = (*Adapter)(nil)
	_ authorization.RoleReader            = (*Adapter)(nil)
	_ authorization.GrantManager          = (*Adapter)(nil)
	_ authorization.GrantReader           = (*Adapter)(nil)
	_ authorization.GroupManager          = (*Adapter)(nil)
	_ authorization.GroupReader           = (*Adapter)(nil)
	_ authorization.WorkflowManager       = (*Adapter)(nil)
	_ authorization.WorkflowReader        = (*Adapter)(nil)
	_ authorization.Reconciler            = (*Adapter)(nil)
	_ authorization.PolicyManager         = (*Adapter)(nil)
	_ authorization.PolicyReader          = (*Adapter)(nil)
	_ authorization.AuditReader           = (*Adapter)(nil)
	_ authorization.DecisionAuditReader   = (*Adapter)(nil)
)

func New(ctx context.Context, catalog *authorization.Catalog, options Options) (*Adapter, error) {
	if catalog == nil {
		return nil, &authorization.Error{Kind: authorization.ErrorInvalidInput, Field: "catalog", Message: "is required"}
	}
	if options.DB == nil {
		return nil, &authorization.Error{Kind: authorization.ErrorInvalidInput, Field: "db", Message: "is required"}
	}
	if strings.TrimSpace(options.InstanceKey) == "" {
		return nil, &authorization.Error{Kind: authorization.ErrorInvalidInput, Field: "instance_key", Message: "is required"}
	}
	if err := options.DB.PingContext(ctx); err != nil {
		return nil, &authorization.Error{Kind: authorization.ErrorUnavailable, Field: "db", Message: "ping failed"}
	}
	adapter := &Adapter{
		catalog: catalog,
		options: options.Memory,
		store:   stateStore{db: options.DB, instanceKey: options.InstanceKey},
	}
	stored, err := adapter.store.load(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		memory, createErr := authorization.NewMemory(catalog, options.Memory)
		if createErr != nil {
			return nil, createErr
		}
		if saveErr := adapter.store.save(ctx, catalog.Version(), catalog.Digest(), memory.RepositorySnapshot()); saveErr != nil {
			return nil, unavailable("bootstrap state", saveErr)
		}
		if projectionErr := memory.RebuildCasbinProjection(); projectionErr != nil {
			return nil, projectionErr
		}
		adapter.memory = memory
		adapter.created = true
		return adapter, nil
	}
	if err != nil {
		return nil, unavailable("load state", err)
	}
	if stored.SchemaVersion != CurrentSchemaVersion {
		return nil, &authorization.Error{
			Kind: authorization.ErrorUnavailable, Field: "schema_version",
			Message: fmt.Sprintf("database has %d, module requires %d", stored.SchemaVersion, CurrentSchemaVersion),
		}
	}
	if stored.CatalogVersion != catalog.Version() || stored.CatalogDigest != catalog.Digest() {
		return nil, &authorization.Error{
			Kind: authorization.ErrorConflict, Field: "catalog",
			Message: "database catalog version or digest does not match the compiled definition",
		}
	}
	memoryOptions := options.Memory
	memoryOptions.RootScopeID = authorization.ScopeID(stored.Snapshot.RootScopeID)
	memoryOptions.ProtectedSubjects = []authorization.SubjectRef{
		{Kind: authorization.SubjectUser, ID: "repository-restore-placeholder"},
	}
	memory, err := authorization.NewMemory(catalog, memoryOptions)
	if err != nil {
		return nil, err
	}
	if err := memory.RestoreRepositorySnapshot(stored.Snapshot); err != nil {
		return nil, err
	}
	if err := memory.RebuildCasbinProjection(); err != nil {
		return nil, err
	}
	adapter.options = memoryOptions
	adapter.memory = memory
	return adapter, nil
}

func (adapter *Adapter) Decide(ctx context.Context, request authorization.DecisionRequest) (authorization.Decision, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Decision, error) {
		return memory.Decide(ctx, request)
	})
}

func (adapter *Adapter) BatchDecide(ctx context.Context, requests []authorization.DecisionRequest) ([]authorization.Decision, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) ([]authorization.Decision, error) {
		return memory.BatchDecide(ctx, requests)
	})
}

func (adapter *Adapter) Plan(ctx context.Context, request authorization.QueryRequest) (authorization.QueryConstraint, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.Plan(ctx, request)
}

func (adapter *Adapter) EffectiveAccess(ctx context.Context, query authorization.EffectiveAccessQuery) (authorization.EffectiveAccess, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.EffectiveAccess(ctx, query)
}

func (adapter *Adapter) ListScopes(ctx context.Context, query authorization.ScopeListQuery) (authorization.ScopePage, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.ListScopes(ctx, query)
}

func (adapter *Adapter) ListGrants(ctx context.Context, query authorization.GrantListQuery) (authorization.GrantPage, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.ListGrants(ctx, query)
}

func (adapter *Adapter) ListGroups(ctx context.Context, query authorization.GroupListQuery) (authorization.GroupPage, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.ListGroups(ctx, query)
}

func (adapter *Adapter) ListRoles(ctx context.Context, query authorization.RoleListQuery) (authorization.RolePage, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.ListRoles(ctx, query)
}

func (adapter *Adapter) ListRequestableRoles(
	ctx context.Context,
	query authorization.RequestableRoleQuery,
) ([]authorization.Role, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.ListRequestableRoles(ctx, query)
}

func (adapter *Adapter) ListApplications(
	ctx context.Context,
	query authorization.ApplicationListQuery,
) (authorization.ApplicationPage, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.ListApplications(ctx, query)
}

func (adapter *Adapter) ListInvitations(
	ctx context.Context,
	query authorization.InvitationListQuery,
) (authorization.InvitationPage, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.ListInvitations(ctx, query)
}

func (adapter *Adapter) ListPolicyRevisions(
	ctx context.Context,
	query authorization.PolicyRevisionListQuery,
) (authorization.PolicyRevisionPage, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.ListPolicyRevisions(ctx, query)
}

func (adapter *Adapter) GetPolicySnapshot(
	ctx context.Context,
	query authorization.PolicySnapshotQuery,
) (authorization.PolicySnapshot, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.GetPolicySnapshot(ctx, query)
}

func (adapter *Adapter) SearchAudit(ctx context.Context, query authorization.AuditQuery) (authorization.AuditPage, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.SearchAudit(ctx, query)
}

func (adapter *Adapter) SearchDecisionAudit(
	ctx context.Context,
	query authorization.DecisionAuditQuery,
) (authorization.DecisionAuditPage, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.SearchDecisionAudit(ctx, query)
}

func (adapter *Adapter) CreateScope(ctx context.Context, command authorization.CreateScopeCommand) (authorization.Scope, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Scope, error) {
		return memory.CreateScope(ctx, command)
	})
}

func (adapter *Adapter) RegisterScope(
	ctx context.Context,
	command authorization.RegisterScopeCommand,
) (authorization.Scope, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Scope, error) {
		return memory.RegisterScope(ctx, command)
	})
}

func (adapter *Adapter) CreateRole(ctx context.Context, command authorization.CreateRoleCommand) (authorization.Role, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Role, error) {
		return memory.CreateRole(ctx, command)
	})
}

func (adapter *Adapter) UpdateRole(ctx context.Context, command authorization.UpdateRoleCommand) (authorization.Role, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Role, error) {
		return memory.UpdateRole(ctx, command)
	})
}

func (adapter *Adapter) RetireRole(ctx context.Context, command authorization.RetireRoleCommand) (authorization.Role, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Role, error) {
		return memory.RetireRole(ctx, command)
	})
}

func (adapter *Adapter) Grant(ctx context.Context, command authorization.GrantCommand) (authorization.Grant, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Grant, error) {
		return memory.Grant(ctx, command)
	})
}

func (adapter *Adapter) Revoke(ctx context.Context, command authorization.RevokeCommand) (authorization.Grant, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Grant, error) {
		return memory.Revoke(ctx, command)
	})
}

func (adapter *Adapter) CreateGroup(ctx context.Context, command authorization.CreateGroupCommand) (authorization.Group, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Group, error) {
		return memory.CreateGroup(ctx, command)
	})
}

func (adapter *Adapter) AddGroupMember(ctx context.Context, command authorization.AddGroupMemberCommand) (authorization.GroupMembership, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.GroupMembership, error) {
		return memory.AddGroupMember(ctx, command)
	})
}

func (adapter *Adapter) RemoveGroupMember(ctx context.Context, command authorization.RemoveGroupMemberCommand) (authorization.GroupMembership, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.GroupMembership, error) {
		return memory.RemoveGroupMember(ctx, command)
	})
}

func (adapter *Adapter) Apply(ctx context.Context, command authorization.ApplyCommand) (authorization.Application, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Application, error) {
		return memory.Apply(ctx, command)
	})
}

func (adapter *Adapter) ReviewApplication(ctx context.Context, command authorization.ReviewApplicationCommand) (authorization.Application, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Application, error) {
		return memory.ReviewApplication(ctx, command)
	})
}

func (adapter *Adapter) WithdrawApplication(ctx context.Context, command authorization.WithdrawApplicationCommand) (authorization.Application, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Application, error) {
		return memory.WithdrawApplication(ctx, command)
	})
}

func (adapter *Adapter) Invite(ctx context.Context, command authorization.InviteCommand) (authorization.InvitationIssue, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.InvitationIssue, error) {
		return memory.Invite(ctx, command)
	})
}

func (adapter *Adapter) AcceptInvitation(ctx context.Context, command authorization.AcceptInvitationCommand) (authorization.Invitation, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Invitation, error) {
		return memory.AcceptInvitation(ctx, command)
	})
}

func (adapter *Adapter) DeclineInvitation(ctx context.Context, command authorization.DeclineInvitationCommand) (authorization.Invitation, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Invitation, error) {
		return memory.DeclineInvitation(ctx, command)
	})
}

func (adapter *Adapter) RevokeInvitation(ctx context.Context, command authorization.RevokeInvitationCommand) (authorization.Invitation, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.Invitation, error) {
		return memory.RevokeInvitation(ctx, command)
	})
}

func (adapter *Adapter) ResendInvitation(ctx context.Context, command authorization.ResendInvitationCommand) (authorization.InvitationIssue, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.InvitationIssue, error) {
		return memory.ResendInvitation(ctx, command)
	})
}

func (adapter *Adapter) HandleEvent(ctx context.Context, event authorization.AutomaticEvent) (authorization.ReconcileResult, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.ReconcileResult, error) {
		return memory.HandleEvent(ctx, event)
	})
}

func (adapter *Adapter) ReconcileSubject(ctx context.Context, command authorization.ReconcileSubjectCommand) (authorization.ReconcileResult, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.ReconcileResult, error) {
		return memory.ReconcileSubject(ctx, command)
	})
}

func (adapter *Adapter) PreviewReconcileSubject(
	ctx context.Context,
	command authorization.ReconcileSubjectCommand,
) (authorization.ReconcileResult, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.PreviewReconcileSubject(ctx, command)
}

func (adapter *Adapter) Backfill(ctx context.Context, commands []authorization.ReconcileSubjectCommand) ([]authorization.ReconcileResult, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) ([]authorization.ReconcileResult, error) {
		return memory.Backfill(ctx, commands)
	})
}

func (adapter *Adapter) CreatePolicyDraft(ctx context.Context, command authorization.CreatePolicyDraftCommand) (authorization.PolicyRevision, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.PolicyRevision, error) {
		return memory.CreatePolicyDraft(ctx, command)
	})
}

func (adapter *Adapter) SetRoleCapabilities(ctx context.Context, command authorization.SetRoleCapabilitiesCommand) (authorization.PolicyRevision, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.PolicyRevision, error) {
		return memory.SetRoleCapabilities(ctx, command)
	})
}

func (adapter *Adapter) SetAccessLayerCapabilities(ctx context.Context, command authorization.SetAccessLayerCapabilitiesCommand) (authorization.PolicyRevision, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.PolicyRevision, error) {
		return memory.SetAccessLayerCapabilities(ctx, command)
	})
}

func (adapter *Adapter) SetAutomaticRuleEnabled(
	ctx context.Context,
	command authorization.SetAutomaticRuleEnabledCommand,
) (authorization.PolicyRevision, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.PolicyRevision, error) {
		return memory.SetAutomaticRuleEnabled(ctx, command)
	})
}

func (adapter *Adapter) ValidatePolicy(ctx context.Context, command authorization.ValidatePolicyCommand) (authorization.PolicyValidation, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.ValidatePolicy(ctx, command)
}

func (adapter *Adapter) PreviewPolicy(ctx context.Context, command authorization.PreviewPolicyCommand) (authorization.PolicyImpact, error) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.memory.PreviewPolicy(ctx, command)
}

func (adapter *Adapter) ActivatePolicy(ctx context.Context, command authorization.ActivatePolicyCommand) (authorization.PolicyRevision, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.PolicyRevision, error) {
		return memory.ActivatePolicy(ctx, command)
	})
}

func (adapter *Adapter) RollbackPolicy(ctx context.Context, command authorization.RollbackPolicyCommand) (authorization.PolicyRevision, error) {
	return mutate(ctx, adapter, func(memory *authorization.Memory) (authorization.PolicyRevision, error) {
		return memory.RollbackPolicy(ctx, command)
	})
}

func mutate[T any](
	ctx context.Context,
	adapter *Adapter,
	operation func(*authorization.Memory) (T, error),
) (T, error) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	var zero T
	clone, err := adapter.cloneMemory()
	if err != nil {
		return zero, err
	}
	result, err := operation(clone)
	if err != nil {
		return zero, err
	}
	if err := clone.RebuildCasbinProjection(); err != nil {
		return zero, err
	}
	if err := adapter.store.save(ctx, adapter.catalog.Version(), adapter.catalog.Digest(), clone.RepositorySnapshot()); err != nil {
		return zero, unavailable("commit state", err)
	}
	adapter.memory = clone
	return result, nil
}

func unavailable(operation string, _ error) error {
	return &authorization.Error{
		Kind: authorization.ErrorUnavailable, Field: "postgres",
		Message: operation + " failed",
	}
}

func (adapter *Adapter) cloneMemory() (*authorization.Memory, error) {
	options := adapter.options
	options.RootScopeID = authorization.ScopeID(adapter.memory.RepositorySnapshot().RootScopeID)
	options.ProtectedSubjects = []authorization.SubjectRef{
		{Kind: authorization.SubjectUser, ID: "repository-clone-placeholder"},
	}
	clone, err := authorization.NewMemory(adapter.catalog, options)
	if err != nil {
		return nil, err
	}
	if err := clone.RestoreRepositorySnapshot(adapter.memory.RepositorySnapshot()); err != nil {
		return nil, err
	}
	if err := clone.RebuildCasbinProjection(); err != nil {
		return nil, err
	}
	return clone, nil
}
