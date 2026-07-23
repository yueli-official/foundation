package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/yueli-official/foundation/go/authorization/internal/repository"
)

type storedInstance struct {
	SchemaVersion  uint
	CatalogVersion uint
	CatalogDigest  string
	Snapshot       repository.Snapshot
}

func (store stateStore) load(ctx context.Context) (storedInstance, error) {
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return storedInstance{}, fmt.Errorf("authorization/postgres: begin load transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	instance, err := store.loadTx(ctx, transaction)
	if err != nil {
		return storedInstance{}, err
	}
	if err := transaction.Commit(); err != nil {
		return storedInstance{}, fmt.Errorf("authorization/postgres: commit load transaction: %w", err)
	}
	return instance, nil
}

func (store stateStore) loadTx(ctx context.Context, transaction *sql.Tx) (storedInstance, error) {
	var instance storedInstance
	var nextID uint64
	err := transaction.QueryRowContext(ctx, `
		SELECT schema_version, catalog_version, catalog_digest,
			root_scope_id, active_policy_revision, next_id
		FROM authorization_instances
		WHERE instance_key = $1
	`, store.instanceKey).Scan(
		&instance.SchemaVersion,
		&instance.CatalogVersion,
		&instance.CatalogDigest,
		&instance.Snapshot.RootScopeID,
		&instance.Snapshot.ActivePolicy,
		&nextID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storedInstance{}, sql.ErrNoRows
		}
		return storedInstance{}, fmt.Errorf("authorization/postgres: load instance: %w", err)
	}
	instance.Snapshot.NextID = nextID

	rows, err := transaction.QueryContext(ctx, `
		SELECT id, scope_type, COALESCE(parent_id, '')
		FROM authorization_scopes
		WHERE instance_key = $1 AND status = 'active'
		ORDER BY depth, id
	`, store.instanceKey)
	if err != nil {
		return storedInstance{}, fmt.Errorf("authorization/postgres: load scopes: %w", err)
	}
	for rows.Next() {
		var scope repository.Scope
		if err := rows.Scan(&scope.ID, &scope.Type, &scope.ParentID); err != nil {
			_ = rows.Close()
			return storedInstance{}, fmt.Errorf("authorization/postgres: scan scope: %w", err)
		}
		instance.Snapshot.Scopes = append(instance.Snapshot.Scopes, scope)
	}
	if err := closeRows(rows); err != nil {
		return storedInstance{}, err
	}

	rows, err = transaction.QueryContext(ctx, `
		SELECT revision, COALESCE(base_revision, 0), state,
			created_by_kind, created_by_id, created_at, activated_at
		FROM authorization_policy_revisions
		WHERE instance_key = $1
		ORDER BY revision
	`, store.instanceKey)
	if err != nil {
		return storedInstance{}, fmt.Errorf("authorization/postgres: load policies: %w", err)
	}
	policyIndex := make(map[uint64]int)
	for rows.Next() {
		var policy repository.Policy
		var activatedAt sql.NullTime
		if err := rows.Scan(
			&policy.Revision.Number,
			&policy.Revision.Base,
			&policy.Revision.State,
			&policy.Revision.CreatedBy.Kind,
			&policy.Revision.CreatedBy.ID,
			&policy.Revision.CreatedAt,
			&activatedAt,
		); err != nil {
			_ = rows.Close()
			return storedInstance{}, fmt.Errorf("authorization/postgres: scan policy: %w", err)
		}
		policy.Revision.ActivatedAt = valueTime(activatedAt)
		policy.AccessLayers = make(map[string][]string)
		policyIndex[policy.Revision.Number] = len(instance.Snapshot.Policies)
		instance.Snapshot.Policies = append(instance.Snapshot.Policies, policy)
		if policy.Revision.Number > instance.Snapshot.NextPolicy {
			instance.Snapshot.NextPolicy = policy.Revision.Number
		}
	}
	if err := closeRows(rows); err != nil {
		return storedInstance{}, err
	}

	rows, err = transaction.QueryContext(ctx, `
		SELECT revision, scope_id
		FROM authorization_policy_scopes
		WHERE instance_key = $1
		ORDER BY revision, scope_id
	`, store.instanceKey)
	if err != nil {
		return storedInstance{}, fmt.Errorf("authorization/postgres: load policy scopes: %w", err)
	}
	for rows.Next() {
		var revision uint64
		var scopeID string
		if err := rows.Scan(&revision, &scopeID); err != nil {
			_ = rows.Close()
			return storedInstance{}, fmt.Errorf("authorization/postgres: scan policy scope: %w", err)
		}
		if index, exists := policyIndex[revision]; exists {
			instance.Snapshot.Policies[index].TouchedScopes = append(
				instance.Snapshot.Policies[index].TouchedScopes,
				scopeID,
			)
		}
	}
	if err := closeRows(rows); err != nil {
		return storedInstance{}, err
	}

	rows, err = transaction.QueryContext(ctx, `
		SELECT rp.revision, rd.id, rd.role_key, rp.display_name, rd.scope_id,
			rd.kind, rp.status, rd.protected, rp.assignment_sources,
			rp.max_duration_seconds
		FROM authorization_role_policies rp
		JOIN authorization_role_definitions rd
			ON rd.instance_key = rp.instance_key AND rd.id = rp.role_id
		WHERE rp.instance_key = $1
		ORDER BY rp.revision, rd.role_key
	`, store.instanceKey)
	if err != nil {
		return storedInstance{}, fmt.Errorf("authorization/postgres: load roles: %w", err)
	}
	roleIndex := make(map[uint64]map[string]int)
	for rows.Next() {
		var revision uint64
		var role repository.Role
		var sources []byte
		var maxDurationSeconds int64
		if err := rows.Scan(
			&revision, &role.ID, &role.Key, &role.DisplayName, &role.ScopeID,
			&role.Kind, &role.Status, &role.Protected, &sources, &maxDurationSeconds,
		); err != nil {
			_ = rows.Close()
			return storedInstance{}, fmt.Errorf("authorization/postgres: scan role: %w", err)
		}
		if err := json.Unmarshal(sources, &role.Sources); err != nil {
			_ = rows.Close()
			return storedInstance{}, fmt.Errorf("authorization/postgres: decode role sources: %w", err)
		}
		role.MaxDuration = time.Duration(maxDurationSeconds) * time.Second
		policyPosition, exists := policyIndex[revision]
		if !exists {
			continue
		}
		if roleIndex[revision] == nil {
			roleIndex[revision] = make(map[string]int)
		}
		roleIndex[revision][role.Key] = len(instance.Snapshot.Policies[policyPosition].Roles)
		instance.Snapshot.Policies[policyPosition].Roles = append(
			instance.Snapshot.Policies[policyPosition].Roles,
			role,
		)
	}
	if err := closeRows(rows); err != nil {
		return storedInstance{}, err
	}

	rows, err = transaction.QueryContext(ctx, `
		SELECT revision, target_kind, target_key, capability_key
		FROM authorization_policy_bindings
		WHERE instance_key = $1
		ORDER BY revision, target_kind, target_key, capability_key
	`, store.instanceKey)
	if err != nil {
		return storedInstance{}, fmt.Errorf("authorization/postgres: load bindings: %w", err)
	}
	for rows.Next() {
		var revision uint64
		var targetKind, targetKey, capability string
		if err := rows.Scan(&revision, &targetKind, &targetKey, &capability); err != nil {
			_ = rows.Close()
			return storedInstance{}, fmt.Errorf("authorization/postgres: scan binding: %w", err)
		}
		policyPosition, exists := policyIndex[revision]
		if !exists {
			continue
		}
		policy := &instance.Snapshot.Policies[policyPosition]
		if targetKind == "access_layer" {
			policy.AccessLayers[targetKey] = append(policy.AccessLayers[targetKey], capability)
			continue
		}
		if rolePosition, exists := roleIndex[revision][targetKey]; exists {
			policy.Roles[rolePosition].Capabilities = append(
				policy.Roles[rolePosition].Capabilities,
				capability,
			)
		}
	}
	if err := closeRows(rows); err != nil {
		return storedInstance{}, err
	}

	rows, err = transaction.QueryContext(ctx, `
		SELECT revision, rule_key, enabled
		FROM authorization_automatic_rules
		WHERE instance_key = $1
		ORDER BY revision, rule_key
	`, store.instanceKey)
	if err != nil {
		return storedInstance{}, fmt.Errorf("authorization/postgres: load automatic rules: %w", err)
	}
	for rows.Next() {
		var revision uint64
		var ruleKey string
		var enabled bool
		if err := rows.Scan(&revision, &ruleKey, &enabled); err != nil {
			_ = rows.Close()
			return storedInstance{}, fmt.Errorf("authorization/postgres: scan automatic rule: %w", err)
		}
		if index, exists := policyIndex[revision]; exists {
			if instance.Snapshot.Policies[index].AutomaticRules == nil {
				instance.Snapshot.Policies[index].AutomaticRules = make(map[string]bool)
			}
			instance.Snapshot.Policies[index].AutomaticRules[ruleKey] = enabled
		}
	}
	if err := closeRows(rows); err != nil {
		return storedInstance{}, err
	}

	if err := store.loadGrants(ctx, transaction, &instance.Snapshot); err != nil {
		return storedInstance{}, err
	}
	if err := store.loadGroups(ctx, transaction, &instance.Snapshot); err != nil {
		return storedInstance{}, err
	}
	if err := store.loadApplications(ctx, transaction, &instance.Snapshot); err != nil {
		return storedInstance{}, err
	}
	if err := store.loadInvitations(ctx, transaction, &instance.Snapshot); err != nil {
		return storedInstance{}, err
	}
	if err := store.loadInbox(ctx, transaction, &instance.Snapshot); err != nil {
		return storedInstance{}, err
	}
	if err := store.loadAudit(ctx, transaction, &instance.Snapshot); err != nil {
		return storedInstance{}, err
	}
	if err := store.loadDecisionAudit(ctx, transaction, &instance.Snapshot); err != nil {
		return storedInstance{}, err
	}
	return instance, nil
}

func (store stateStore) loadGrants(ctx context.Context, tx *sql.Tx, snapshot *repository.Snapshot) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT g.id, g.target_kind, g.target_id, g.role_id, rd.role_key,
			g.scope_id, g.source, g.valid_from, g.expires_at, g.revoked_at
		FROM authorization_grants g
		JOIN authorization_role_definitions rd
			ON rd.instance_key = g.instance_key AND rd.id = g.role_id
		WHERE g.instance_key = $1
		ORDER BY g.id
	`, store.instanceKey)
	if err != nil {
		return fmt.Errorf("authorization/postgres: load grants: %w", err)
	}
	for rows.Next() {
		var grant repository.Grant
		var expiresAt, revokedAt sql.NullTime
		if err := rows.Scan(
			&grant.ID, &grant.Target.Kind, &grant.Target.ID, &grant.RoleID,
			&grant.RoleKey, &grant.ScopeID, &grant.Source, &grant.ValidFrom,
			&expiresAt, &revokedAt,
		); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authorization/postgres: scan grant: %w", err)
		}
		grant.ExpiresAt = valueTime(expiresAt)
		grant.RevokedAt = valueTime(revokedAt)
		snapshot.Grants = append(snapshot.Grants, grant)
	}
	return closeRows(rows)
}

func (store stateStore) loadGroups(ctx context.Context, tx *sql.Tx, snapshot *repository.Snapshot) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, scope_id, display_name
		FROM authorization_groups
		WHERE instance_key = $1 AND status = 'active'
		ORDER BY id
	`, store.instanceKey)
	if err != nil {
		return fmt.Errorf("authorization/postgres: load groups: %w", err)
	}
	groupIndex := make(map[string]int)
	for rows.Next() {
		var group repository.Group
		if err := rows.Scan(&group.ID, &group.ScopeID, &group.DisplayName); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authorization/postgres: scan group: %w", err)
		}
		groupIndex[group.ID] = len(snapshot.Groups)
		snapshot.Groups = append(snapshot.Groups, group)
	}
	if err := closeRows(rows); err != nil {
		return err
	}
	rows, err = tx.QueryContext(ctx, `
		SELECT group_id, subject_kind, subject_id
		FROM authorization_group_members
		WHERE instance_key = $1 AND removed_at IS NULL
		ORDER BY group_id, subject_kind, subject_id
	`, store.instanceKey)
	if err != nil {
		return fmt.Errorf("authorization/postgres: load group members: %w", err)
	}
	for rows.Next() {
		var groupID string
		var member repository.Subject
		if err := rows.Scan(&groupID, &member.Kind, &member.ID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authorization/postgres: scan group member: %w", err)
		}
		if index, exists := groupIndex[groupID]; exists {
			snapshot.Groups[index].Members = append(snapshot.Groups[index].Members, member)
		}
	}
	return closeRows(rows)
}

func (store stateStore) loadApplications(ctx context.Context, tx *sql.Tx, snapshot *repository.Snapshot) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT a.id, a.request_group_id, a.subject_kind, a.subject_id,
			a.role_id, rd.role_key, a.scope_id, a.reason, a.state, a.grant_id,
			a.created_at, a.reviewed_at, a.reviewed_by_kind,
			a.reviewed_by_id, a.review_reason, a.idempotency_key
		FROM authorization_applications a
		JOIN authorization_role_definitions rd
			ON rd.instance_key = a.instance_key AND rd.id = a.role_id
		WHERE a.instance_key = $1
		ORDER BY a.id
	`, store.instanceKey)
	if err != nil {
		return fmt.Errorf("authorization/postgres: load applications: %w", err)
	}
	for rows.Next() {
		var record repository.Application
		var requestGroupID, grantID, reviewedKind, reviewedID, reviewReason, idempotencyKey sql.NullString
		var reviewedAt sql.NullTime
		if err := rows.Scan(
			&record.ID, &requestGroupID, &record.Subject.Kind, &record.Subject.ID,
			&record.RoleID, &record.RoleKey, &record.ScopeID, &record.Reason,
			&record.State, &grantID, &record.CreatedAt, &reviewedAt,
			&reviewedKind, &reviewedID, &reviewReason, &idempotencyKey,
		); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authorization/postgres: scan application: %w", err)
		}
		record.RequestGroupID = requestGroupID.String
		record.GrantID = grantID.String
		record.ReviewedAt = valueTime(reviewedAt)
		record.ReviewedBy = repository.Subject{Kind: reviewedKind.String, ID: reviewedID.String}
		record.ReviewReason = reviewReason.String
		record.IdempotencyKey = idempotencyKey.String
		snapshot.Applications = append(snapshot.Applications, record)
	}
	return closeRows(rows)
}

func (store stateStore) loadInvitations(ctx context.Context, tx *sql.Tx, snapshot *repository.Snapshot) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT i.id, i.subject_kind, i.subject_id, i.normalized_email,
			i.role_id, rd.role_key, i.scope_id, i.token_digest, i.state,
			i.invited_by_kind, i.invited_by_id, i.accepted_by_kind,
			i.accepted_by_id, i.grant_id, i.created_at, i.expires_at,
			i.completed_at
		FROM authorization_invitations i
		JOIN authorization_role_definitions rd
			ON rd.instance_key = i.instance_key AND rd.id = i.role_id
		WHERE i.instance_key = $1
		ORDER BY i.id
	`, store.instanceKey)
	if err != nil {
		return fmt.Errorf("authorization/postgres: load invitations: %w", err)
	}
	for rows.Next() {
		var record repository.Invitation
		var subjectKind, subjectID, email, acceptedKind, acceptedID, grantID sql.NullString
		var completedAt sql.NullTime
		if err := rows.Scan(
			&record.ID, &subjectKind, &subjectID, &email, &record.RoleID,
			&record.RoleKey, &record.ScopeID, &record.TokenDigest, &record.State,
			&record.InvitedBy.Kind, &record.InvitedBy.ID, &acceptedKind,
			&acceptedID, &grantID, &record.CreatedAt, &record.ExpiresAt,
			&completedAt,
		); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authorization/postgres: scan invitation: %w", err)
		}
		record.Subject = repository.Subject{Kind: subjectKind.String, ID: subjectID.String}
		record.Email = email.String
		record.AcceptedBy = repository.Subject{Kind: acceptedKind.String, ID: acceptedID.String}
		record.GrantID = grantID.String
		record.CompletedAt = valueTime(completedAt)
		snapshot.Invitations = append(snapshot.Invitations, record)
	}
	return closeRows(rows)
}

func (store stateStore) loadInbox(ctx context.Context, tx *sql.Tx, snapshot *repository.Snapshot) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT event_id, result
		FROM authorization_inbox_events
		WHERE instance_key = $1
		ORDER BY event_id
	`, store.instanceKey)
	if err != nil {
		return fmt.Errorf("authorization/postgres: load inbox: %w", err)
	}
	for rows.Next() {
		var record repository.InboxEvent
		var result []byte
		if err := rows.Scan(&record.ID, &result); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authorization/postgres: scan inbox event: %w", err)
		}
		if err := json.Unmarshal(result, &record.Result); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authorization/postgres: decode inbox result: %w", err)
		}
		snapshot.Inbox = append(snapshot.Inbox, record)
	}
	return closeRows(rows)
}

func (store stateStore) loadAudit(ctx context.Context, tx *sql.Tx, snapshot *repository.Snapshot) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, action, actor_kind, actor_id, subject_kind, subject_id,
			role_key, scope_id, policy_revision, correlation_id, occurred_at
		FROM authorization_audit_events
		WHERE instance_key = $1
		ORDER BY occurred_at, id
	`, store.instanceKey)
	if err != nil {
		return fmt.Errorf("authorization/postgres: load audit: %w", err)
	}
	for rows.Next() {
		var record repository.AuditEvent
		var actorKind, actorID, subjectKind, subjectID, roleKey, scopeID, correlationID sql.NullString
		if err := rows.Scan(
			&record.ID, &record.Action, &actorKind, &actorID, &subjectKind,
			&subjectID, &roleKey, &scopeID, &record.PolicyRevision,
			&correlationID, &record.OccurredAt,
		); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authorization/postgres: scan audit event: %w", err)
		}
		record.Actor = repository.Subject{Kind: actorKind.String, ID: actorID.String}
		record.Subject = repository.Subject{Kind: subjectKind.String, ID: subjectID.String}
		record.RoleKey = roleKey.String
		record.ScopeID = scopeID.String
		record.CorrelationID = correlationID.String
		snapshot.Audit = append(snapshot.Audit, record)
	}
	return closeRows(rows)
}

func (store stateStore) loadDecisionAudit(
	ctx context.Context,
	tx *sql.Tx,
	snapshot *repository.Snapshot,
) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT decision_id, subject_kind, subject_id, capability_key, scope_id,
			resource_type, resource_id, resource_revision, allowed, reason,
			constraint_key, policy_revision, sources, correlation_id, occurred_at
		FROM authorization_decision_events
		WHERE instance_key = $1
		ORDER BY occurred_at, decision_id
	`, store.instanceKey)
	if err != nil {
		return fmt.Errorf("authorization/postgres: load decision audit: %w", err)
	}
	for rows.Next() {
		var record repository.DecisionAuditEvent
		var subjectID, resourceType, resourceID, resourceRevision, constraint, correlationID sql.NullString
		var sources []byte
		if err := rows.Scan(
			&record.DecisionID, &record.Subject.Kind, &subjectID, &record.Capability,
			&record.ScopeID, &resourceType, &resourceID, &resourceRevision,
			&record.Allowed, &record.Reason, &constraint, &record.PolicyRevision,
			&sources, &correlationID, &record.OccurredAt,
		); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authorization/postgres: scan decision event: %w", err)
		}
		record.Subject.ID = subjectID.String
		record.ResourceType = resourceType.String
		record.ResourceID = resourceID.String
		record.ResourceRevision = resourceRevision.String
		record.Constraint = constraint.String
		record.CorrelationID = correlationID.String
		if err := json.Unmarshal(sources, &record.Sources); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authorization/postgres: decode decision sources: %w", err)
		}
		snapshot.DecisionAudit = append(snapshot.DecisionAudit, record)
	}
	return closeRows(rows)
}

func closeRows(rows *sql.Rows) error {
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("authorization/postgres: iterate rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("authorization/postgres: close rows: %w", err)
	}
	return nil
}

func valueTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}
