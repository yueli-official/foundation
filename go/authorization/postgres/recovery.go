package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/yueli-official/foundation/go/authorization"
	"github.com/yueli-official/foundation/go/authorization/internal/repository"
)

type RecoveryCommand struct {
	DB           *sql.DB
	InstanceKey  string
	Target       authorization.SubjectRef
	DryRun       bool
	Confirmation string
}

type RecoveryResult struct {
	WouldCreate               bool
	GrantID                   authorization.GrantID
	AuditID                   authorization.AuditID
	RequiresProjectionRebuild bool
}

// RecoverProtectedAdministrator is an offline disaster-recovery operation. It
// only works when no protected administrator Grant remains active.
func RecoverProtectedAdministrator(
	ctx context.Context,
	command RecoveryCommand,
) (RecoveryResult, error) {
	if command.DB == nil {
		return RecoveryResult{}, &authorization.Error{Kind: authorization.ErrorInvalidInput, Field: "db", Message: "is required"}
	}
	if strings.TrimSpace(command.InstanceKey) == "" {
		return RecoveryResult{}, &authorization.Error{Kind: authorization.ErrorInvalidInput, Field: "instance_key", Message: "is required"}
	}
	if (command.Target.Kind != authorization.SubjectUser && command.Target.Kind != authorization.SubjectService) ||
		strings.TrimSpace(command.Target.ID) == "" {
		return RecoveryResult{}, &authorization.Error{Kind: authorization.ErrorInvalidInput, Field: "target", Message: "must be an identified user or service"}
	}
	expectedConfirmation := command.InstanceKey + ":" + string(command.Target.Kind) + ":" + command.Target.ID
	if !command.DryRun && command.Confirmation != expectedConfirmation {
		return RecoveryResult{}, &authorization.Error{
			Kind: authorization.ErrorInvalidInput, Field: "confirmation",
			Message: "does not exactly match the recovery target",
		}
	}
	auditBridge, err := newAuthorizationAuditBridge(ctx, command.DB, command.InstanceKey)
	if err != nil {
		return RecoveryResult{}, unavailable("initialize recovery audit", err)
	}
	tx, err := command.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return RecoveryResult{}, unavailable("begin recovery", err)
	}
	defer func() { _ = tx.Rollback() }()
	var rootScopeID string
	var activeRevision uint64
	var nextID uint64
	if err := tx.QueryRowContext(ctx, `
		SELECT root_scope_id, active_policy_revision, next_id
		FROM authorization_instances
		WHERE instance_key = $1
		FOR UPDATE
	`, command.InstanceKey).Scan(&rootScopeID, &activeRevision, &nextID); err != nil {
		if err == sql.ErrNoRows {
			return RecoveryResult{}, &authorization.Error{Kind: authorization.ErrorNotFound, Field: "instance_key", Message: "instance not found"}
		}
		return RecoveryResult{}, unavailable("lock recovery instance", err)
	}
	var roleID, roleKey string
	if err := tx.QueryRowContext(ctx, `
		SELECT id, role_key
		FROM authorization_role_definitions
		WHERE instance_key = $1 AND protected = TRUE
	`, command.InstanceKey).Scan(&roleID, &roleKey); err != nil {
		return RecoveryResult{}, unavailable("find protected role", err)
	}
	var activeProtected int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*)
		FROM authorization_grants g
		JOIN authorization_role_definitions r
			ON r.instance_key = g.instance_key AND r.id = g.role_id
		WHERE g.instance_key = $1 AND r.protected = TRUE
			AND g.revoked_at IS NULL
			AND g.valid_from <= now()
			AND (g.expires_at IS NULL OR g.expires_at > now())
	`, command.InstanceKey).Scan(&activeProtected); err != nil {
		return RecoveryResult{}, unavailable("count protected grants", err)
	}
	if activeProtected != 0 {
		return RecoveryResult{}, &authorization.Error{
			Kind: authorization.ErrorInvariant, Field: "instance_key",
			Message: "recovery is only available when no protected administrator is active",
		}
	}
	result := RecoveryResult{WouldCreate: true, RequiresProjectionRebuild: true}
	if command.DryRun {
		return result, nil
	}
	nextID += 2
	grantID := authorization.GrantID(fmt.Sprintf("grant-%d", nextID-1))
	auditID := authorization.AuditID(fmt.Sprintf("audit-%d", nextID))
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO authorization_grants (
			instance_key, id, target_kind, target_id, role_id, scope_id,
			source, valid_from, created_by_kind, created_by_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'service', 'offline-recovery', $8)
	`, command.InstanceKey, grantID, command.Target.Kind, command.Target.ID,
		roleID, rootScopeID, authorization.GrantSourceRecovery, now); err != nil {
		return RecoveryResult{}, unavailable("insert recovery grant", err)
	}
	if err := auditBridge.append(ctx, tx, []repository.AuditEvent{{
		ID: string(auditID), Action: string(authorization.AuditRecoveryProtected),
		Actor:   repository.Subject{Kind: string(authorization.SubjectService), ID: "offline-recovery"},
		Subject: repository.Subject{Kind: string(command.Target.Kind), ID: command.Target.ID},
		RoleKey: roleKey, ScopeID: rootScopeID, PolicyRevision: activeRevision,
		OccurredAt: now,
	}}, nil); err != nil {
		return RecoveryResult{}, unavailable("append recovery audit", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE authorization_instances
		SET next_id = $2, updated_at = $3
		WHERE instance_key = $1
	`, command.InstanceKey, nextID, now); err != nil {
		return RecoveryResult{}, unavailable("advance recovery id", err)
	}
	if err := tx.Commit(); err != nil {
		return RecoveryResult{}, unavailable("commit recovery", err)
	}
	result.GrantID = grantID
	result.AuditID = auditID
	return result, nil
}
