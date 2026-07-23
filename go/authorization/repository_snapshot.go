package authorization

import (
	"fmt"
	"sort"

	"github.com/yueli-official/foundation/go/authorization/internal/repository"
)

// RepositorySnapshot is an Adapter-only seam. Its internal parameter type
// prevents consumers outside this module tree from depending on persistence
// representation.
func (module *Memory) RepositorySnapshot() repository.Snapshot {
	module.mu.RLock()
	defer module.mu.RUnlock()
	return module.repositorySnapshotLocked()
}

func (module *Memory) repositorySnapshotLocked() repository.Snapshot {
	snapshot := repository.Snapshot{
		RootScopeID:  string(module.rootScopeIDLocked()),
		ActivePolicy: module.activePolicy,
		NextPolicy:   module.nextPolicy,
		NextID:       module.nextID,
	}
	for capability := range module.catalog.capabilities {
		snapshot.CatalogCapabilities = append(snapshot.CatalogCapabilities, string(capability))
	}
	for _, rule := range module.catalog.automaticRules {
		snapshot.AutomaticRules = append(snapshot.AutomaticRules, repository.AutomaticRule{
			Key: rule.Key, Trigger: string(rule.Trigger), Predicate: string(rule.Predicate),
			RoleKey: string(rule.Role), Enabled: rule.Enabled,
		})
	}
	for _, scope := range module.scopes {
		snapshot.Scopes = append(snapshot.Scopes, repository.Scope{
			ID: string(scope.ID), Type: string(scope.Type), ParentID: string(scope.ParentID),
		})
	}
	for _, policy := range module.policies {
		record := repository.Policy{
			Revision: repository.PolicyRevision{
				Number: policy.revision.Number, Base: policy.revision.Base, State: string(policy.revision.State),
				CreatedBy: toRepositorySubject(policy.revision.CreatedBy),
				CreatedAt: policy.revision.CreatedAt, ActivatedAt: policy.revision.ActivatedAt,
			},
			AccessLayers:   make(map[string][]string, len(policy.accessLayers)),
			AutomaticRules: make(map[string]bool, len(policy.automaticRules)),
		}
		for _, role := range policy.roles {
			roleRecord := repository.Role{
				ID: string(role.ID), Key: string(role.Key), DisplayName: role.DisplayName,
				ScopeID: string(role.ScopeID), Kind: string(role.Kind), Status: string(role.Status),
				Protected: role.Protected, MaxDuration: role.Assignment.MaxDuration,
			}
			for _, capability := range role.Capabilities {
				roleRecord.Capabilities = append(roleRecord.Capabilities, string(capability))
			}
			for _, source := range role.Assignment.Sources {
				roleRecord.Sources = append(roleRecord.Sources, string(source))
			}
			record.Roles = append(record.Roles, roleRecord)
		}
		for layer, capabilities := range policy.accessLayers {
			for _, capability := range capabilities {
				record.AccessLayers[string(layer)] = append(record.AccessLayers[string(layer)], string(capability))
			}
		}
		for rule, enabled := range policy.automaticRules {
			record.AutomaticRules[rule] = enabled
		}
		for scopeID := range policy.touchedScopes {
			record.TouchedScopes = append(record.TouchedScopes, string(scopeID))
		}
		snapshot.Policies = append(snapshot.Policies, record)
	}
	for _, grant := range module.grants {
		snapshot.Grants = append(snapshot.Grants, toRepositoryGrant(grant))
	}
	for _, group := range module.groups {
		record := repository.Group{ID: string(group.ID), ScopeID: string(group.ScopeID), DisplayName: group.DisplayName}
		for _, member := range module.groupMembers[group.ID] {
			record.Members = append(record.Members, toRepositorySubject(member))
		}
		snapshot.Groups = append(snapshot.Groups, record)
	}
	for _, application := range module.applications {
		snapshot.Applications = append(snapshot.Applications, repository.Application{
			ID: string(application.ID), RequestGroupID: string(application.RequestGroupID),
			Subject: toRepositorySubject(application.Subject), RoleID: string(application.RoleID),
			RoleKey: string(application.Role), ScopeID: string(application.ScopeID), Reason: application.Reason,
			State: string(application.State), GrantID: string(application.GrantID), CreatedAt: application.CreatedAt,
			ReviewedAt: application.ReviewedAt, ReviewedBy: toRepositorySubject(application.ReviewedBy),
			ReviewReason: application.ReviewReason, IdempotencyKey: application.IdempotencyKey,
		})
	}
	for _, invitation := range module.invitations {
		record := repository.Invitation{
			ID: string(invitation.ID), Subject: toRepositorySubject(invitation.Subject), Email: invitation.Email,
			RoleID: string(invitation.RoleID), RoleKey: string(invitation.Role), ScopeID: string(invitation.ScopeID),
			State: string(invitation.State), InvitedBy: toRepositorySubject(invitation.InvitedBy),
			AcceptedBy: toRepositorySubject(invitation.AcceptedBy), GrantID: string(invitation.GrantID),
			CreatedAt: invitation.CreatedAt, ExpiresAt: invitation.ExpiresAt, CompletedAt: invitation.CompletedAt,
		}
		for digest, invitationID := range module.invitationTokens {
			if invitationID == invitation.ID {
				record.TokenDigest = append([]byte(nil), digest[:]...)
				break
			}
		}
		snapshot.Invitations = append(snapshot.Invitations, record)
	}
	for id, result := range module.inbox {
		record := repository.InboxEvent{
			ID: id,
			Result: repository.ReconcileResult{
				Subject: toRepositorySubject(result.Subject),
				Created: result.Created,
			},
		}
		for _, grant := range result.Grants {
			record.Result.Grants = append(record.Result.Grants, toRepositoryGrant(grant))
		}
		snapshot.Inbox = append(snapshot.Inbox, record)
	}
	for _, event := range module.audit {
		snapshot.Audit = append(snapshot.Audit, repository.AuditEvent{
			ID: string(event.ID), Action: string(event.Action), Actor: toRepositorySubject(event.Actor),
			Subject: toRepositorySubject(event.Subject), RoleKey: string(event.Role),
			ScopeID: string(event.ScopeID), PolicyRevision: event.PolicyRevision,
			CorrelationID: event.CorrelationID, OccurredAt: event.OccurredAt,
		})
	}
	for _, event := range module.decisionAudit {
		record := repository.DecisionAuditEvent{
			DecisionID: string(event.DecisionID), Subject: toRepositorySubject(event.Subject),
			Capability: string(event.Capability), ScopeID: string(event.ScopeID),
			ResourceType: string(event.ResourceType), ResourceID: string(event.ResourceID),
			ResourceRevision: event.ResourceRevision, Allowed: event.Allowed, Reason: string(event.Reason),
			Constraint: string(event.Constraint), PolicyRevision: event.PolicyRevision,
			CorrelationID: event.CorrelationID,
			OccurredAt:    event.OccurredAt,
		}
		for _, source := range event.Sources {
			record.Sources = append(record.Sources, repository.DecisionSource{
				Kind: string(source.Kind), RoleID: string(source.RoleID), RoleKey: string(source.Role),
				AccessLayer: string(source.AccessLayer), GrantID: string(source.GrantID),
				GroupID: string(source.GroupID),
			})
		}
		snapshot.DecisionAudit = append(snapshot.DecisionAudit, record)
	}
	sortRepositorySnapshot(&snapshot)
	return snapshot
}

// RestoreRepositorySnapshot replaces state after a durable Adapter has
// validated schema and catalog compatibility.
func (module *Memory) RestoreRepositorySnapshot(snapshot repository.Snapshot) error {
	module.mu.Lock()
	defer module.mu.Unlock()
	if snapshot.RootScopeID == "" || snapshot.ActivePolicy == 0 {
		return &Error{Kind: ErrorInvalidInput, Field: "snapshot", Message: "root scope and active policy are required"}
	}
	scopes := make(map[ScopeID]Scope, len(snapshot.Scopes))
	for _, record := range snapshot.Scopes {
		scope := Scope{ID: ScopeID(record.ID), Type: ScopeType(record.Type), ParentID: ScopeID(record.ParentID)}
		scopes[scope.ID] = scope
	}
	if _, exists := scopes[ScopeID(snapshot.RootScopeID)]; !exists {
		return &Error{Kind: ErrorInvalidInput, Field: "snapshot.scopes", Message: "root scope is missing"}
	}
	policies := make(map[uint64]*memoryPolicy, len(snapshot.Policies))
	for _, record := range snapshot.Policies {
		policy := &memoryPolicy{
			revision: PolicyRevision{
				Number: record.Revision.Number, Base: record.Revision.Base, State: PolicyState(record.Revision.State),
				CreatedBy: fromRepositorySubject(record.Revision.CreatedBy),
				CreatedAt: record.Revision.CreatedAt, ActivatedAt: record.Revision.ActivatedAt,
			},
			roles:          make(map[RoleKey]Role, len(record.Roles)),
			accessLayers:   make(map[AccessLayerKey][]CapabilityKey, len(record.AccessLayers)),
			automaticRules: make(map[string]bool, len(record.AutomaticRules)),
			touchedScopes:  make(map[ScopeID]struct{}, len(record.TouchedScopes)),
		}
		for _, roleRecord := range record.Roles {
			role := Role{
				ID: RoleID(roleRecord.ID), Key: RoleKey(roleRecord.Key), DisplayName: roleRecord.DisplayName,
				ScopeID: ScopeID(roleRecord.ScopeID), Kind: RoleKind(roleRecord.Kind),
				Status: RoleStatus(roleRecord.Status), Protected: roleRecord.Protected,
				Assignment: AssignmentPolicy{MaxDuration: roleRecord.MaxDuration},
			}
			for _, capability := range roleRecord.Capabilities {
				role.Capabilities = append(role.Capabilities, CapabilityKey(capability))
			}
			for _, source := range roleRecord.Sources {
				role.Assignment.Sources = append(role.Assignment.Sources, GrantSource(source))
			}
			policy.roles[role.Key] = role
		}
		for layer, capabilities := range record.AccessLayers {
			for _, capability := range capabilities {
				policy.accessLayers[AccessLayerKey(layer)] = append(
					policy.accessLayers[AccessLayerKey(layer)],
					CapabilityKey(capability),
				)
			}
		}
		for rule, enabled := range record.AutomaticRules {
			policy.automaticRules[rule] = enabled
		}
		for _, scopeID := range record.TouchedScopes {
			policy.touchedScopes[ScopeID(scopeID)] = struct{}{}
		}
		policies[policy.revision.Number] = policy
	}
	if policy, exists := policies[snapshot.ActivePolicy]; !exists || policy.revision.State != PolicyActive {
		return &Error{Kind: ErrorInvalidInput, Field: "snapshot.policies", Message: "active policy is missing"}
	}
	module.scopes = scopes
	module.policies = policies
	module.activePolicy = snapshot.ActivePolicy
	module.nextPolicy = snapshot.NextPolicy
	module.nextID = snapshot.NextID
	module.grants = make(map[GrantID]Grant, len(snapshot.Grants))
	for _, record := range snapshot.Grants {
		grant := fromRepositoryGrant(record)
		module.grants[grant.ID] = grant
	}
	module.groups = make(map[GroupID]Group, len(snapshot.Groups))
	module.groupMembers = make(map[GroupID]map[string]SubjectRef, len(snapshot.Groups))
	for _, record := range snapshot.Groups {
		group := Group{ID: GroupID(record.ID), ScopeID: ScopeID(record.ScopeID), DisplayName: record.DisplayName}
		module.groups[group.ID] = group
		module.groupMembers[group.ID] = make(map[string]SubjectRef, len(record.Members))
		for _, memberRecord := range record.Members {
			member := fromRepositorySubject(memberRecord)
			module.groupMembers[group.ID][subjectKey(member)] = member
		}
	}
	module.applications = make(map[ApplicationID]Application, len(snapshot.Applications))
	for _, record := range snapshot.Applications {
		application := Application{
			ID: ApplicationID(record.ID), RequestGroupID: RequestGroupID(record.RequestGroupID),
			Subject: fromRepositorySubject(record.Subject), RoleID: RoleID(record.RoleID),
			Role: RoleKey(record.RoleKey), ScopeID: ScopeID(record.ScopeID), Reason: record.Reason,
			State: ApplicationState(record.State), GrantID: GrantID(record.GrantID), CreatedAt: record.CreatedAt,
			ReviewedAt: record.ReviewedAt, ReviewedBy: fromRepositorySubject(record.ReviewedBy),
			ReviewReason: record.ReviewReason, IdempotencyKey: record.IdempotencyKey,
		}
		module.applications[application.ID] = application
	}
	module.invitations = make(map[InvitationID]Invitation, len(snapshot.Invitations))
	module.invitationTokens = make(map[[32]byte]InvitationID, len(snapshot.Invitations))
	for _, record := range snapshot.Invitations {
		invitation := Invitation{
			ID: InvitationID(record.ID), Subject: fromRepositorySubject(record.Subject), Email: record.Email,
			RoleID: RoleID(record.RoleID), Role: RoleKey(record.RoleKey), ScopeID: ScopeID(record.ScopeID),
			State: InvitationState(record.State), InvitedBy: fromRepositorySubject(record.InvitedBy),
			AcceptedBy: fromRepositorySubject(record.AcceptedBy), GrantID: GrantID(record.GrantID),
			CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt, CompletedAt: record.CompletedAt,
		}
		module.invitations[invitation.ID] = invitation
		if len(record.TokenDigest) != 0 {
			if len(record.TokenDigest) != 32 {
				return &Error{Kind: ErrorInvalidInput, Field: "snapshot.invitations.token_digest", Message: "must be 32 bytes"}
			}
			var digest [32]byte
			copy(digest[:], record.TokenDigest)
			module.invitationTokens[digest] = invitation.ID
		}
	}
	module.inbox = make(map[string]ReconcileResult, len(snapshot.Inbox))
	for _, record := range snapshot.Inbox {
		result := ReconcileResult{Subject: fromRepositorySubject(record.Result.Subject), Created: record.Result.Created}
		for _, grant := range record.Result.Grants {
			result.Grants = append(result.Grants, fromRepositoryGrant(grant))
		}
		module.inbox[record.ID] = result
	}
	module.audit = make([]AuditEvent, 0, len(snapshot.Audit))
	for _, record := range snapshot.Audit {
		module.audit = append(module.audit, AuditEvent{
			ID: AuditID(record.ID), Action: AuditAction(record.Action),
			Actor: fromRepositorySubject(record.Actor), Subject: fromRepositorySubject(record.Subject),
			Role: RoleKey(record.RoleKey), ScopeID: ScopeID(record.ScopeID),
			PolicyRevision: record.PolicyRevision, CorrelationID: record.CorrelationID,
			OccurredAt: record.OccurredAt,
		})
	}
	module.decisionAudit = make([]DecisionAuditEvent, 0, len(snapshot.DecisionAudit))
	for _, record := range snapshot.DecisionAudit {
		event := DecisionAuditEvent{
			DecisionID: DecisionID(record.DecisionID), Subject: fromRepositorySubject(record.Subject),
			Capability: CapabilityKey(record.Capability), ScopeID: ScopeID(record.ScopeID),
			ResourceType: ResourceType(record.ResourceType), ResourceID: ResourceID(record.ResourceID),
			ResourceRevision: record.ResourceRevision, Allowed: record.Allowed, Reason: ReasonCode(record.Reason),
			Constraint: ConstraintKey(record.Constraint), PolicyRevision: record.PolicyRevision,
			CorrelationID: record.CorrelationID,
			OccurredAt:    record.OccurredAt,
		}
		for _, source := range record.Sources {
			event.Sources = append(event.Sources, DecisionSource{
				Kind: ReasonCode(source.Kind), RoleID: RoleID(source.RoleID), Role: RoleKey(source.RoleKey),
				AccessLayer: AccessLayerKey(source.AccessLayer), GrantID: GrantID(source.GrantID),
				GroupID: GroupID(source.GroupID),
			})
		}
		module.decisionAudit = append(module.decisionAudit, event)
	}
	if module.nextPolicy < module.activePolicy {
		return &Error{Kind: ErrorInvalidInput, Field: "snapshot.next_policy", Message: fmt.Sprintf("is behind active revision %d", module.activePolicy)}
	}
	return nil
}

func toRepositorySubject(subject SubjectRef) repository.Subject {
	return repository.Subject{Kind: string(subject.Kind), ID: subject.ID}
}

func fromRepositorySubject(subject repository.Subject) SubjectRef {
	return SubjectRef{Kind: SubjectKind(subject.Kind), ID: subject.ID}
}

func toRepositoryGrant(grant Grant) repository.Grant {
	return repository.Grant{
		ID: string(grant.ID), Target: toRepositorySubject(grant.Target), RoleID: string(grant.RoleID),
		RoleKey: string(grant.Role), ScopeID: string(grant.ScopeID), Source: string(grant.Source),
		ValidFrom: grant.ValidFrom, ExpiresAt: grant.ExpiresAt, RevokedAt: grant.RevokedAt,
	}
}

func fromRepositoryGrant(grant repository.Grant) Grant {
	return Grant{
		ID: GrantID(grant.ID), Target: fromRepositorySubject(grant.Target), RoleID: RoleID(grant.RoleID),
		Role: RoleKey(grant.RoleKey), ScopeID: ScopeID(grant.ScopeID), Source: GrantSource(grant.Source),
		ValidFrom: grant.ValidFrom, ExpiresAt: grant.ExpiresAt, RevokedAt: grant.RevokedAt,
	}
}

func sortRepositorySnapshot(snapshot *repository.Snapshot) {
	sort.Strings(snapshot.CatalogCapabilities)
	sort.Slice(snapshot.AutomaticRules, func(i, j int) bool {
		return snapshot.AutomaticRules[i].Key < snapshot.AutomaticRules[j].Key
	})
	sort.Slice(snapshot.Scopes, func(i, j int) bool { return snapshot.Scopes[i].ID < snapshot.Scopes[j].ID })
	sort.Slice(snapshot.Policies, func(i, j int) bool {
		return snapshot.Policies[i].Revision.Number < snapshot.Policies[j].Revision.Number
	})
	for index := range snapshot.Policies {
		policy := &snapshot.Policies[index]
		sort.Slice(policy.Roles, func(i, j int) bool { return policy.Roles[i].Key < policy.Roles[j].Key })
		sort.Strings(policy.TouchedScopes)
	}
	sort.Slice(snapshot.Grants, func(i, j int) bool { return snapshot.Grants[i].ID < snapshot.Grants[j].ID })
	sort.Slice(snapshot.Groups, func(i, j int) bool { return snapshot.Groups[i].ID < snapshot.Groups[j].ID })
	sort.Slice(snapshot.Applications, func(i, j int) bool { return snapshot.Applications[i].ID < snapshot.Applications[j].ID })
	sort.Slice(snapshot.Invitations, func(i, j int) bool { return snapshot.Invitations[i].ID < snapshot.Invitations[j].ID })
	sort.Slice(snapshot.Inbox, func(i, j int) bool { return snapshot.Inbox[i].ID < snapshot.Inbox[j].ID })
	sort.Slice(snapshot.Audit, func(i, j int) bool { return snapshot.Audit[i].ID < snapshot.Audit[j].ID })
	sort.Slice(snapshot.DecisionAudit, func(i, j int) bool {
		return snapshot.DecisionAudit[i].DecisionID < snapshot.DecisionAudit[j].DecisionID
	})
}
