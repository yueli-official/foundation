package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yueli-official/foundation/go/authorization/internal/repository"
)

type stateStore struct {
	db          *sql.DB
	instanceKey string
}

func (store stateStore) save(
	ctx context.Context,
	catalogVersion uint,
	catalogDigest string,
	snapshot repository.Snapshot,
) error {
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("authorization/postgres: begin state transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	if err := store.saveTx(ctx, transaction, catalogVersion, catalogDigest, snapshot); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("authorization/postgres: commit state transaction: %w", err)
	}
	return nil
}

func (store stateStore) saveTx(
	ctx context.Context,
	transaction *sql.Tx,
	catalogVersion uint,
	catalogDigest string,
	snapshot repository.Snapshot,
) error {
	now := time.Now().UTC()
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO authorization_instances (
			instance_key, schema_version, catalog_version, catalog_digest,
			root_scope_id, active_policy_revision, next_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		ON CONFLICT (instance_key) DO UPDATE SET
			schema_version = EXCLUDED.schema_version,
			catalog_version = EXCLUDED.catalog_version,
			catalog_digest = EXCLUDED.catalog_digest,
			root_scope_id = EXCLUDED.root_scope_id,
			active_policy_revision = EXCLUDED.active_policy_revision,
			next_id = EXCLUDED.next_id,
			updated_at = EXCLUDED.updated_at
	`, store.instanceKey, CurrentSchemaVersion, catalogVersion, catalogDigest,
		snapshot.RootScopeID, snapshot.ActivePolicy, snapshot.NextID, now); err != nil {
		return fmt.Errorf("authorization/postgres: save instance: %w", err)
	}

	for _, scope := range snapshot.Scopes {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO authorization_scopes (
				instance_key, id, scope_type, parent_id, path, depth, status, created_at
			) VALUES ($1, $2, $3, NULL, $2, 0, 'active', $4)
			ON CONFLICT (instance_key, id) DO UPDATE SET
				scope_type = EXCLUDED.scope_type,
				status = 'active',
				retired_at = NULL
		`, store.instanceKey, scope.ID, scope.Type, now); err != nil {
			return fmt.Errorf("authorization/postgres: save scope %q: %w", scope.ID, err)
		}
	}
	for _, scope := range snapshot.Scopes {
		path, depth, err := scopePath(snapshot.Scopes, scope.ID)
		if err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, `
			UPDATE authorization_scopes
			SET parent_id = $3, path = $4, depth = $5
			WHERE instance_key = $1 AND id = $2
		`, store.instanceKey, scope.ID, nullString(scope.ParentID), path, depth); err != nil {
			return fmt.Errorf("authorization/postgres: link scope %q: %w", scope.ID, err)
		}
	}

	for _, policy := range snapshot.Policies {
		revision := policy.Revision
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO authorization_policy_revisions (
				instance_key, revision, base_revision, state, created_by_kind,
				created_by_id, created_at, activated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (instance_key, revision) DO UPDATE SET
				state = EXCLUDED.state,
				activated_at = EXCLUDED.activated_at
		`, store.instanceKey, revision.Number, nullUint64(revision.Base), revision.State,
			revision.CreatedBy.Kind, revision.CreatedBy.ID, revision.CreatedAt,
			nullTime(revision.ActivatedAt)); err != nil {
			return fmt.Errorf("authorization/postgres: save policy %d: %w", revision.Number, err)
		}
		for _, scopeID := range policy.TouchedScopes {
			if _, err := transaction.ExecContext(ctx, `
				INSERT INTO authorization_policy_scopes (instance_key, revision, scope_id)
				VALUES ($1, $2, $3)
				ON CONFLICT DO NOTHING
			`, store.instanceKey, revision.Number, scopeID); err != nil {
				return fmt.Errorf("authorization/postgres: save policy scope %q: %w", scopeID, err)
			}
		}
	}

	roleDefinitions := make(map[string]repository.Role)
	roleCreatedAt := make(map[string]time.Time)
	for _, policy := range snapshot.Policies {
		for _, role := range policy.Roles {
			roleDefinitions[role.ID] = role
			if existing, ok := roleCreatedAt[role.ID]; !ok || policy.Revision.CreatedAt.Before(existing) {
				roleCreatedAt[role.ID] = policy.Revision.CreatedAt
			}
		}
	}
	for _, role := range roleDefinitions {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO authorization_role_definitions (
				instance_key, id, role_key, scope_id, kind, protected, created_at, retired_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (instance_key, id) DO UPDATE SET
				role_key = EXCLUDED.role_key,
				scope_id = EXCLUDED.scope_id,
				retired_at = EXCLUDED.retired_at
		`, store.instanceKey, role.ID, role.Key, role.ScopeID, role.Kind, role.Protected,
			roleCreatedAt[role.ID], retiredAt(role, now)); err != nil {
			return fmt.Errorf("authorization/postgres: save role definition %q: %w", role.Key, err)
		}
	}

	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM authorization_policy_bindings WHERE instance_key = $1
	`, store.instanceKey); err != nil {
		return fmt.Errorf("authorization/postgres: replace policy bindings: %w", err)
	}
	for _, policy := range snapshot.Policies {
		for _, role := range policy.Roles {
			roleSources := role.Sources
			if roleSources == nil {
				roleSources = []string{}
			}
			sources, err := json.Marshal(roleSources)
			if err != nil {
				return fmt.Errorf("authorization/postgres: encode role sources: %w", err)
			}
			if _, err := transaction.ExecContext(ctx, `
				INSERT INTO authorization_role_policies (
					instance_key, revision, role_id, display_name, status,
					assignment_sources, max_duration_seconds
				) VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (instance_key, revision, role_id) DO UPDATE SET
					display_name = EXCLUDED.display_name,
					status = EXCLUDED.status,
					assignment_sources = EXCLUDED.assignment_sources,
					max_duration_seconds = EXCLUDED.max_duration_seconds
			`, store.instanceKey, policy.Revision.Number, role.ID, role.DisplayName, role.Status,
				sources, int64(role.MaxDuration/time.Second)); err != nil {
				return fmt.Errorf("authorization/postgres: save role policy %q: %w", role.Key, err)
			}
			for _, capability := range role.Capabilities {
				if err := insertBinding(ctx, transaction, store.instanceKey, policy.Revision.Number, "role", role.Key, capability); err != nil {
					return err
				}
			}
		}
		for layer, capabilities := range policy.AccessLayers {
			for _, capability := range capabilities {
				if err := insertBinding(ctx, transaction, store.instanceKey, policy.Revision.Number, "access_layer", layer, capability); err != nil {
					return err
				}
			}
		}
	}
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM authorization_automatic_rules WHERE instance_key = $1
	`, store.instanceKey); err != nil {
		return fmt.Errorf("authorization/postgres: replace automatic rules: %w", err)
	}
	for _, policy := range snapshot.Policies {
		rolesByKey := make(map[string]repository.Role, len(policy.Roles))
		for _, role := range policy.Roles {
			rolesByKey[role.Key] = role
		}
		for _, rule := range snapshot.AutomaticRules {
			role, exists := rolesByKey[rule.RoleKey]
			if !exists {
				return fmt.Errorf("authorization/postgres: automatic rule %q role %q is missing", rule.Key, rule.RoleKey)
			}
			if _, err := transaction.ExecContext(ctx, `
				INSERT INTO authorization_automatic_rules (
					instance_key, revision, rule_key, trigger_key, predicate_key,
					role_id, scope_id, enabled, parameters
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '{}'::jsonb)
			`, store.instanceKey, policy.Revision.Number, rule.Key, rule.Trigger,
				rule.Predicate, role.ID, snapshot.RootScopeID, policy.AutomaticRules[rule.Key]); err != nil {
				return fmt.Errorf("authorization/postgres: save automatic rule %q: %w", rule.Key, err)
			}
		}
	}

	for _, group := range snapshot.Groups {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO authorization_groups (
				instance_key, id, scope_id, display_name, status, created_at
			) VALUES ($1, $2, $3, $4, 'active', $5)
			ON CONFLICT (instance_key, id) DO UPDATE SET
				scope_id = EXCLUDED.scope_id,
				display_name = EXCLUDED.display_name,
				status = 'active',
				retired_at = NULL
		`, store.instanceKey, group.ID, group.ScopeID, group.DisplayName, now); err != nil {
			return fmt.Errorf("authorization/postgres: save group %q: %w", group.ID, err)
		}
		if err := store.syncGroupMembers(ctx, transaction, group, now); err != nil {
			return err
		}
	}

	for _, grant := range snapshot.Grants {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO authorization_grants (
				instance_key, id, target_kind, target_id, role_id, scope_id,
				source, valid_from, expires_at, revoked_at, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (instance_key, id) DO UPDATE SET
				expires_at = EXCLUDED.expires_at,
				revoked_at = EXCLUDED.revoked_at
		`, store.instanceKey, grant.ID, grant.Target.Kind, grant.Target.ID, grant.RoleID,
			grant.ScopeID, grant.Source, grant.ValidFrom, nullTime(grant.ExpiresAt),
			nullTime(grant.RevokedAt), grant.ValidFrom); err != nil {
			return fmt.Errorf("authorization/postgres: save grant %q: %w", grant.ID, err)
		}
	}

	for _, application := range snapshot.Applications {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO authorization_applications (
				instance_key, id, request_group_id, subject_kind, subject_id,
				role_id, scope_id, reason, state, grant_id, created_at,
				reviewed_at, reviewed_by_kind, reviewed_by_id, review_reason,
				idempotency_key
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			ON CONFLICT (instance_key, id) DO UPDATE SET
				state = EXCLUDED.state,
				grant_id = EXCLUDED.grant_id,
				reviewed_at = EXCLUDED.reviewed_at,
				reviewed_by_kind = EXCLUDED.reviewed_by_kind,
				reviewed_by_id = EXCLUDED.reviewed_by_id,
				review_reason = EXCLUDED.review_reason
		`, store.instanceKey, application.ID, nullString(application.RequestGroupID),
			application.Subject.Kind, application.Subject.ID, application.RoleID,
			application.ScopeID, application.Reason, application.State,
			nullString(application.GrantID), application.CreatedAt,
			nullTime(application.ReviewedAt), nullString(application.ReviewedBy.Kind),
			nullString(application.ReviewedBy.ID), nullString(application.ReviewReason),
			nullString(application.IdempotencyKey)); err != nil {
			return fmt.Errorf("authorization/postgres: save application %q: %w", application.ID, err)
		}
	}

	for _, invitation := range snapshot.Invitations {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO authorization_invitations (
				instance_key, id, subject_kind, subject_id, normalized_email,
				role_id, scope_id, token_digest, state, invited_by_kind,
				invited_by_id, accepted_by_kind, accepted_by_id, grant_id,
				created_at, expires_at, completed_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
			ON CONFLICT (instance_key, id) DO UPDATE SET
				token_digest = EXCLUDED.token_digest,
				state = EXCLUDED.state,
				accepted_by_kind = EXCLUDED.accepted_by_kind,
				accepted_by_id = EXCLUDED.accepted_by_id,
				grant_id = EXCLUDED.grant_id,
				expires_at = EXCLUDED.expires_at,
				completed_at = EXCLUDED.completed_at
		`, store.instanceKey, invitation.ID, nullString(invitation.Subject.Kind),
			nullString(invitation.Subject.ID), nullString(invitation.Email),
			invitation.RoleID, invitation.ScopeID, invitation.TokenDigest, invitation.State,
			invitation.InvitedBy.Kind, invitation.InvitedBy.ID,
			nullString(invitation.AcceptedBy.Kind), nullString(invitation.AcceptedBy.ID),
			nullString(invitation.GrantID), invitation.CreatedAt, invitation.ExpiresAt,
			nullTime(invitation.CompletedAt)); err != nil {
			return fmt.Errorf("authorization/postgres: save invitation %q: %w", invitation.ID, err)
		}
	}

	for _, event := range snapshot.Inbox {
		result, err := json.Marshal(event.Result)
		if err != nil {
			return fmt.Errorf("authorization/postgres: encode inbox result: %w", err)
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO authorization_inbox_events (
				instance_key, source, event_id, subject_kind, subject_id,
				policy_revision, result, handled_at
			) VALUES ($1, 'identity', $2, $3, $4, $5, $6, $7)
			ON CONFLICT (instance_key, source, event_id) DO NOTHING
		`, store.instanceKey, event.ID, event.Result.Subject.Kind, event.Result.Subject.ID,
			snapshot.ActivePolicy, result, now); err != nil {
			return fmt.Errorf("authorization/postgres: save inbox event %q: %w", event.ID, err)
		}
	}

	for _, event := range snapshot.Audit {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO authorization_audit_events (
				instance_key, id, action, actor_kind, actor_id, subject_kind,
				subject_id, role_key, scope_id, policy_revision, correlation_id, occurred_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (instance_key, id) DO NOTHING
		`, store.instanceKey, event.ID, event.Action, nullString(event.Actor.Kind),
			nullString(event.Actor.ID), nullString(event.Subject.Kind), nullString(event.Subject.ID),
			nullString(event.RoleKey), nullString(event.ScopeID), event.PolicyRevision,
			nullString(event.CorrelationID), event.OccurredAt); err != nil {
			return fmt.Errorf("authorization/postgres: save audit event %q: %w", event.ID, err)
		}
	}
	for _, event := range snapshot.DecisionAudit {
		sources, err := json.Marshal(event.Sources)
		if err != nil {
			return fmt.Errorf("authorization/postgres: encode decision sources: %w", err)
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO authorization_decision_events (
				instance_key, decision_id, subject_kind, subject_id,
				capability_key, scope_id, resource_type, resource_id,
				resource_revision, allowed, reason, constraint_key,
				policy_revision, sources, correlation_id, occurred_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			ON CONFLICT (instance_key, decision_id) DO NOTHING
		`, store.instanceKey, event.DecisionID, event.Subject.Kind, nullString(event.Subject.ID),
			event.Capability, event.ScopeID, nullString(event.ResourceType),
			nullString(event.ResourceID), nullString(event.ResourceRevision), event.Allowed,
			event.Reason, nullString(event.Constraint), event.PolicyRevision, sources,
			nullString(event.CorrelationID), event.OccurredAt); err != nil {
			return fmt.Errorf("authorization/postgres: save decision event %q: %w", event.DecisionID, err)
		}
	}
	if err := store.rebuildProjectionTx(ctx, transaction, snapshot); err != nil {
		return err
	}
	return nil
}

func (store stateStore) rebuildProjection(ctx context.Context, snapshot repository.Snapshot) error {
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("authorization/postgres: begin projection transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	if err := store.rebuildProjectionTx(ctx, transaction, snapshot); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("authorization/postgres: commit projection transaction: %w", err)
	}
	return nil
}

func (store stateStore) rebuildProjectionTx(
	ctx context.Context,
	transaction *sql.Tx,
	snapshot repository.Snapshot,
) error {
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM authorization_projection_rules WHERE instance_key = $1
	`, store.instanceKey); err != nil {
		return fmt.Errorf("authorization/postgres: clear execution projection: %w", err)
	}
	var active *repository.Policy
	for index := range snapshot.Policies {
		if snapshot.Policies[index].Revision.Number == snapshot.ActivePolicy {
			active = &snapshot.Policies[index]
			break
		}
	}
	if active == nil {
		return fmt.Errorf("authorization/postgres: active policy %d is missing", snapshot.ActivePolicy)
	}
	for _, role := range active.Roles {
		if role.Status != "active" {
			continue
		}
		capabilities := role.Capabilities
		if role.Protected {
			capabilities = snapshot.CatalogCapabilities
		}
		for _, capability := range capabilities {
			if err := store.insertProjectionRule(
				ctx, transaction, snapshot.ActivePolicy, "permission", "",
				role.ID, capability, role.ScopeID, map[string]string{"role_key": role.Key},
			); err != nil {
				return err
			}
		}
	}
	for layer, capabilities := range active.AccessLayers {
		for _, capability := range capabilities {
			if err := store.insertProjectionRule(
				ctx, transaction, snapshot.ActivePolicy, "access_layer", layer,
				layer, capability, snapshot.RootScopeID, nil,
			); err != nil {
				return err
			}
		}
	}
	groupMembers := make(map[string][]repository.Subject, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		groupMembers[group.ID] = group.Members
	}
	activeRoles := make(map[string]repository.Role, len(active.Roles))
	for _, role := range active.Roles {
		if role.Status == "active" {
			activeRoles[role.ID] = role
		}
	}
	for _, grant := range snapshot.Grants {
		role, exists := activeRoles[grant.RoleID]
		if !exists {
			continue
		}
		subjects := []repository.Subject{grant.Target}
		if grant.Target.Kind == "group" {
			subjects = groupMembers[grant.Target.ID]
		}
		for _, scope := range snapshot.Scopes {
			if !scopeContainsRecords(snapshot.Scopes, grant.ScopeID, scope.ID) {
				continue
			}
			for _, subject := range subjects {
				provenance := map[string]string{"grant_id": grant.ID}
				if grant.Target.Kind == "group" {
					provenance["group_id"] = grant.Target.ID
				}
				if err := store.insertProjectionRule(
					ctx, transaction, snapshot.ActivePolicy, "membership",
					subject.Kind+":"+subject.ID, role.ID, "", scope.ID, provenance,
				); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (store stateStore) insertProjectionRule(
	ctx context.Context,
	transaction *sql.Tx,
	revision uint64,
	ruleKind, subjectKey, roleKey, capabilityKey, scopeID string,
	provenance map[string]string,
) error {
	if provenance == nil {
		provenance = map[string]string{}
	}
	encoded, err := json.Marshal(provenance)
	if err != nil {
		return fmt.Errorf("authorization/postgres: encode projection provenance: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO authorization_projection_rules (
			instance_key, policy_revision, rule_kind, subject_key, role_key,
			capability_key, scope_id, provenance
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT DO NOTHING
	`, store.instanceKey, revision, ruleKind, subjectKey, roleKey, capabilityKey, scopeID, encoded); err != nil {
		return fmt.Errorf("authorization/postgres: insert projection rule: %w", err)
	}
	return nil
}

func (store stateStore) syncGroupMembers(
	ctx context.Context,
	transaction *sql.Tx,
	group repository.Group,
	now time.Time,
) error {
	desired := make(map[string]repository.Subject, len(group.Members))
	for _, member := range group.Members {
		desired[member.Kind+"\x00"+member.ID] = member
	}
	rows, err := transaction.QueryContext(ctx, `
		SELECT subject_kind, subject_id
		FROM authorization_group_members
		WHERE instance_key = $1 AND group_id = $2 AND removed_at IS NULL
	`, store.instanceKey, group.ID)
	if err != nil {
		return fmt.Errorf("authorization/postgres: read group members %q: %w", group.ID, err)
	}
	var active []repository.Subject
	for rows.Next() {
		var member repository.Subject
		if err := rows.Scan(&member.Kind, &member.ID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authorization/postgres: scan group member: %w", err)
		}
		active = append(active, member)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("authorization/postgres: close group members: %w", err)
	}
	for _, member := range active {
		key := member.Kind + "\x00" + member.ID
		if _, keep := desired[key]; keep {
			delete(desired, key)
			continue
		}
		if _, err := transaction.ExecContext(ctx, `
			UPDATE authorization_group_members
			SET removed_at = $5, removed_by_kind = 'system', removed_by_id = 'snapshot'
			WHERE instance_key = $1 AND group_id = $2 AND subject_kind = $3
				AND subject_id = $4 AND removed_at IS NULL
		`, store.instanceKey, group.ID, member.Kind, member.ID, now); err != nil {
			return fmt.Errorf("authorization/postgres: remove group member: %w", err)
		}
	}
	for _, member := range desired {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO authorization_group_members (
				instance_key, group_id, subject_kind, subject_id,
				added_at, added_by_kind, added_by_id
			) VALUES ($1, $2, $3, $4, $5, 'system', 'snapshot')
		`, store.instanceKey, group.ID, member.Kind, member.ID, now); err != nil {
			return fmt.Errorf("authorization/postgres: add group member: %w", err)
		}
	}
	return nil
}

func insertBinding(
	ctx context.Context,
	transaction *sql.Tx,
	instanceKey string,
	revision uint64,
	targetKind, targetKey, capability string,
) error {
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO authorization_policy_bindings (
			instance_key, revision, target_kind, target_key, capability_key
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT DO NOTHING
	`, instanceKey, revision, targetKind, targetKey, capability); err != nil {
		return fmt.Errorf("authorization/postgres: save policy binding: %w", err)
	}
	return nil
}

func scopePath(scopes []repository.Scope, scopeID string) (string, int, error) {
	byID := make(map[string]repository.Scope, len(scopes))
	for _, scope := range scopes {
		byID[scope.ID] = scope
	}
	visited := make(map[string]struct{}, len(scopes))
	path := ""
	current := scopeID
	depth := -1
	for current != "" {
		if _, cycle := visited[current]; cycle {
			return "", 0, fmt.Errorf("authorization/postgres: scope cycle at %q", current)
		}
		visited[current] = struct{}{}
		scope, exists := byID[current]
		if !exists {
			return "", 0, fmt.Errorf("authorization/postgres: missing scope %q", current)
		}
		path = "/" + current + path
		depth++
		current = scope.ParentID
	}
	return path, depth, nil
}

func scopeContainsRecords(scopes []repository.Scope, ancestorID, descendantID string) bool {
	byID := make(map[string]repository.Scope, len(scopes))
	for _, scope := range scopes {
		byID[scope.ID] = scope
	}
	visited := make(map[string]struct{}, len(scopes))
	current := descendantID
	for current != "" {
		if current == ancestorID {
			return true
		}
		if _, cycle := visited[current]; cycle {
			return false
		}
		visited[current] = struct{}{}
		scope, exists := byID[current]
		if !exists {
			return false
		}
		current = scope.ParentID
	}
	return false
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullUint64(value uint64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func retiredAt(role repository.Role, now time.Time) any {
	if role.Status == "retired" {
		return now
	}
	return nil
}
