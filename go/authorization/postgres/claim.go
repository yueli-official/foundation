package postgres

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/yueli-official/foundation/go/authorization"
	"github.com/yueli-official/foundation/go/authorization/internal/repository"
	"github.com/yueli-official/foundation/go/identifier"
)

func (adapter *Adapter) AdministratorClaimStatus(ctx context.Context) (authorization.AdministratorClaimStatus, error) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	claimed, _, err := adapter.claimState(ctx, adapter.store.db)
	if err != nil {
		return authorization.AdministratorClaimStatus{}, unavailable("read administrator claim status", err)
	}
	if claimed {
		memoryStatus, memoryErr := adapter.memory.AdministratorClaimStatus(ctx)
		if memoryErr != nil {
			return authorization.AdministratorClaimStatus{}, memoryErr
		}
		if !memoryStatus.Claimed {
			if err := adapter.reloadClaimedState(ctx); err != nil {
				return authorization.AdministratorClaimStatus{}, err
			}
		}
	}
	return authorization.AdministratorClaimStatus{Claimed: claimed}, nil
}

func (adapter *Adapter) ClaimInitialAdministrator(
	ctx context.Context,
	command authorization.ClaimInitialAdministratorCommand,
) (authorization.ClaimInitialAdministratorResult, error) {
	if command.Actor.Kind != authorization.SubjectUser || strings.TrimSpace(command.Actor.ID) == "" {
		return authorization.ClaimInitialAdministratorResult{}, &authorization.Error{
			Kind: authorization.ErrorInvalidInput, Field: "actor", Message: "only an identified user may claim an instance",
		}
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	tx, err := adapter.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return authorization.ClaimInitialAdministratorResult{}, unavailable("begin administrator claim", err)
	}
	defer func() { _ = tx.Rollback() }()
	var rootScopeID string
	var activeRevision, nextID uint64
	if err := tx.QueryRowContext(ctx, `
		SELECT root_scope_id, active_policy_revision, next_id
		FROM authorization_instances
		WHERE instance_key = $1
		FOR UPDATE
	`, adapter.store.instanceKey).Scan(&rootScopeID, &activeRevision, &nextID); err != nil {
		if err == sql.ErrNoRows {
			return authorization.ClaimInitialAdministratorResult{}, &authorization.Error{
				Kind: authorization.ErrorNotFound, Field: "instance", Message: "authorization instance not found",
			}
		}
		return authorization.ClaimInitialAdministratorResult{}, unavailable("lock administrator claim", err)
	}
	claimed, existing, err := adapter.claimState(ctx, tx)
	if err != nil {
		return authorization.ClaimInitialAdministratorResult{}, unavailable("inspect administrator claim", err)
	}
	if claimed {
		_ = tx.Rollback()
		if err := adapter.reloadClaimedState(ctx); err != nil {
			return authorization.ClaimInitialAdministratorResult{}, err
		}
		if existing.ID != "" && existing.Target == command.Actor && existing.Source == authorization.GrantSourceInitialClaim {
			return authorization.ClaimInitialAdministratorResult{
				Status: authorization.AdministratorClaimStatus{Claimed: true}, Grant: existing,
			}, nil
		}
		return authorization.ClaimInitialAdministratorResult{}, &authorization.Error{
			Kind: authorization.ErrorConflict, Field: "instance", Message: "initial administrator has already been claimed",
		}
	}
	var roleID, roleKey string
	if err := tx.QueryRowContext(ctx, `
		SELECT id, role_key
		FROM authorization_role_definitions
		WHERE instance_key = $1 AND protected = TRUE AND retired_at IS NULL
	`, adapter.store.instanceKey).Scan(&roleID, &roleKey); err != nil {
		return authorization.ClaimInitialAdministratorResult{}, unavailable("find protected role", err)
	}
	now := time.Now().UTC()
	grant := authorization.Grant{
		ID: authorization.GrantID(identifier.MustNew().String()), Target: command.Actor,
		RoleID: authorization.RoleID(roleID), Role: authorization.RoleKey(roleKey),
		ScopeID: authorization.ScopeID(rootScopeID), Source: authorization.GrantSourceInitialClaim, ValidFrom: now,
	}
	auditID := authorization.AuditID(identifier.MustNew().String())
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO authorization_grants (
			instance_key, id, target_kind, target_id, role_id, scope_id,
			source, valid_from, created_by_kind, created_by_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $3, $4, $8)
	`, adapter.store.instanceKey, grant.ID, command.Actor.Kind, command.Actor.ID,
		roleID, rootScopeID, grant.Source, now); err != nil {
		return authorization.ClaimInitialAdministratorResult{}, unavailable("insert administrator claim grant", err)
	}
	if err := adapter.audit.append(ctx, tx, []repository.AuditEvent{{
		ID: string(auditID), Action: string(authorization.AuditInitialAdministratorClaimed),
		Actor:   repository.Subject{Kind: string(command.Actor.Kind), ID: command.Actor.ID},
		Subject: repository.Subject{Kind: string(command.Actor.Kind), ID: command.Actor.ID},
		RoleKey: roleKey, ScopeID: rootScopeID, PolicyRevision: activeRevision, OccurredAt: now,
	}}, nil); err != nil {
		return authorization.ClaimInitialAdministratorResult{}, unavailable("append administrator claim audit", err)
	}
	nextID += 2
	if _, err := tx.ExecContext(ctx, `
		UPDATE authorization_instances SET next_id = $2, updated_at = $3 WHERE instance_key = $1
	`, adapter.store.instanceKey, nextID, now); err != nil {
		return authorization.ClaimInitialAdministratorResult{}, unavailable("advance administrator claim state", err)
	}
	if err := tx.Commit(); err != nil {
		return authorization.ClaimInitialAdministratorResult{}, unavailable("commit administrator claim", err)
	}
	if err := adapter.reloadClaimedState(ctx); err != nil {
		return authorization.ClaimInitialAdministratorResult{}, err
	}
	return authorization.ClaimInitialAdministratorResult{
		Status: authorization.AdministratorClaimStatus{Claimed: true}, Grant: grant, Created: true,
	}, nil
}

type claimQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (adapter *Adapter) claimState(
	ctx context.Context,
	query claimQuerier,
) (bool, authorization.Grant, error) {
	var grant authorization.Grant
	var kind, targetID, roleID, roleKey, scopeID, source string
	var validFrom time.Time
	err := query.QueryRowContext(ctx, `
		SELECT g.id, g.target_kind, g.target_id, g.role_id, r.role_key,
			g.scope_id, g.source, g.valid_from
		FROM authorization_grants g
		JOIN authorization_role_definitions r
			ON r.instance_key = g.instance_key AND r.id = g.role_id
		WHERE g.instance_key = $1 AND r.protected = TRUE
			AND g.revoked_at IS NULL AND g.valid_from <= now()
			AND (g.expires_at IS NULL OR g.expires_at > now())
		ORDER BY g.created_at ASC
		LIMIT 1
	`, adapter.store.instanceKey).Scan(
		&grant.ID, &kind, &targetID, &roleID, &roleKey, &scopeID, &source, &validFrom,
	)
	if err == nil {
		grant.Target = authorization.SubjectRef{Kind: authorization.SubjectKind(kind), ID: targetID}
		grant.RoleID, grant.Role, grant.ScopeID = authorization.RoleID(roleID), authorization.RoleKey(roleKey), authorization.ScopeID(scopeID)
		grant.Source, grant.ValidFrom = authorization.GrantSource(source), validFrom
		return true, grant, nil
	}
	if err != sql.ErrNoRows {
		return false, authorization.Grant{}, err
	}
	var claimed bool
	if err := query.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM authorization_grants g
			JOIN authorization_role_definitions r
				ON r.instance_key = g.instance_key AND r.id = g.role_id
			WHERE g.instance_key = $1 AND r.protected = TRUE
				AND g.source IN ($2, $3, $4)
		)
	`, adapter.store.instanceKey,
		authorization.GrantSourceBootstrap,
		authorization.GrantSourceInitialClaim,
		authorization.GrantSourceRecovery,
	).Scan(&claimed); err != nil {
		return false, authorization.Grant{}, err
	}
	return claimed, authorization.Grant{}, nil
}

func (adapter *Adapter) reloadClaimedState(ctx context.Context) error {
	stored, err := adapter.store.load(ctx)
	if err != nil {
		return unavailable("reload administrator claim", err)
	}
	memory, err := adapter.memoryFromSnapshot(stored.Snapshot)
	if err != nil {
		return err
	}
	if err := adapter.store.rebuildProjection(ctx, memory.RepositorySnapshot()); err != nil {
		return unavailable("rebuild administrator claim projection", err)
	}
	adapter.memory = memory
	return nil
}
