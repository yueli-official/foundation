package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/yueli-official/foundation/go/audit"
	"github.com/yueli-official/foundation/go/authorization"
	"github.com/yueli-official/foundation/go/authorization/internal/repository"
)

const (
	authorizationAuditRetention    audit.RetentionClass = "retention.authorization_management"
	authorizationDecisionRetention audit.RetentionClass = "retention.authorization_decision"
	authorizationDecisionAction    audit.ActionName     = "authorization.decision.evaluated"
)

var authorizationManagementActions = []authorization.AuditAction{
	authorization.AuditBootstrapProtected,
	authorization.AuditScopeCreated,
	authorization.AuditScopeRegistered,
	authorization.AuditScopeReparented,
	authorization.AuditGroupCreated,
	authorization.AuditGroupMemberAdded,
	authorization.AuditGroupMemberRemoved,
	authorization.AuditGrantCreated,
	authorization.AuditGrantRevoked,
	authorization.AuditApplicationCreated,
	authorization.AuditApplicationReviewed,
	authorization.AuditApplicationWithdrawn,
	authorization.AuditInvitationCreated,
	authorization.AuditInvitationAccepted,
	authorization.AuditInvitationDeclined,
	authorization.AuditInvitationRevoked,
	authorization.AuditInvitationResent,
	authorization.AuditAutomaticReconciled,
	authorization.AuditRoleCreated,
	authorization.AuditRoleUpdated,
	authorization.AuditRoleRetired,
	authorization.AuditAutomaticRuleChanged,
	authorization.AuditPolicyDraftCreated,
	authorization.AuditPolicyBindingsChanged,
	authorization.AuditPolicyActivated,
	authorization.AuditPolicyRolledBack,
	authorization.AuditRecoveryProtected,
}

type managementEvidence struct {
	SubjectKind    string
	Role           string
	Scope          string
	PolicyRevision uint64
}

type decisionEvidence struct {
	SubjectKind      string
	Capability       string
	Scope            string
	ResourceType     string
	ResourceRevision string
	Allowed          bool
	Reason           string
	Constraint       string
	PolicyRevision   uint64
	Sources          []repository.DecisionSource
}

type authorizationAuditBridge struct {
	journal    *audit.Postgres
	management map[authorization.AuditAction]audit.Contract[managementEvidence]
	decision   audit.Contract[decisionEvidence]
}

func newAuthorizationAuditBridge(
	ctx context.Context,
	db *sql.DB,
	instanceKey string,
) (*authorizationAuditBridge, error) {
	definition := authorizationAuditDefinition()
	catalog, err := audit.Compile(definition)
	if err != nil {
		return nil, err
	}
	journal, err := audit.NewPostgres(ctx, catalog, audit.PostgresOptions{
		DB: db, InstanceKey: "authorization:" + instanceKey,
		Source:             audit.Source{Service: "authorization", Module: "authorization", Instance: instanceKey},
		EnableMirrorOutbox: true,
	})
	if err != nil {
		return nil, err
	}
	bridge := &authorizationAuditBridge{
		journal:    journal,
		management: make(map[authorization.AuditAction]audit.Contract[managementEvidence], len(authorizationManagementActions)),
	}
	for _, legacyAction := range authorizationManagementActions {
		contract, err := audit.BindAction(catalog, managementAuditAction(legacyAction), encodeManagementEvidence)
		if err != nil {
			return nil, err
		}
		bridge.management[legacyAction] = contract
	}
	bridge.decision, err = audit.BindAction(catalog, audit.Action{Name: authorizationDecisionAction, Version: 1}, encodeDecisionEvidence)
	if err != nil {
		return nil, err
	}
	return bridge, nil
}

func authorizationAuditDefinition() audit.Definition {
	commonManagementEvidence := []audit.FieldDefinition{
		{Key: "authorization.subject.kind", Kind: audit.EvidenceCode},
		{Key: "authorization.role", Kind: audit.EvidenceCode, MaxLength: 256},
		{Key: "authorization.scope", Kind: audit.EvidenceReference, MaxLength: 512},
		{Key: "authorization.policy_revision", Kind: audit.EvidenceCount, Required: true},
	}
	actions := make([]audit.ActionDefinition, 0, len(authorizationManagementActions)+1)
	for _, legacyAction := range authorizationManagementActions {
		actions = append(actions, audit.ActionDefinition{
			Action: managementAuditAction(legacyAction), Category: audit.CategoryAuthorization,
			TargetTypes: []string{"authorization.event", "authorization.subject", "authorization.role", "authorization.scope"},
			Commit:      audit.CommitAtomicRequired, Retention: authorizationAuditRetention,
			Evidence: slices.Clone(commonManagementEvidence),
		})
	}
	actions = append(actions, audit.ActionDefinition{
		Action:      audit.Action{Name: authorizationDecisionAction, Version: 1},
		Category:    audit.CategoryAuthorization,
		TargetTypes: []string{"authorization.decision", "authorization.resource", "authorization.scope"},
		Commit:      audit.CommitAtomicRequired, Retention: authorizationDecisionRetention,
		Evidence: []audit.FieldDefinition{
			{Key: "decision.subject.kind", Kind: audit.EvidenceCode, Required: true},
			{Key: "decision.capability", Kind: audit.EvidenceCode, Required: true, MaxLength: 256},
			{Key: "decision.scope", Kind: audit.EvidenceReference, Required: true, MaxLength: 512},
			{Key: "decision.resource.type", Kind: audit.EvidenceCode, MaxLength: 256},
			{Key: "decision.resource.revision", Kind: audit.EvidenceReference, MaxLength: 512},
			{Key: "decision.allowed", Kind: audit.EvidenceBool, Required: true},
			{Key: "decision.reason", Kind: audit.EvidenceCode, Required: true, MaxLength: 256},
			{Key: "decision.constraint", Kind: audit.EvidenceCode, MaxLength: 256},
			{Key: "decision.policy_revision", Kind: audit.EvidenceCount, Required: true},
			{Key: "decision.source.kinds", Kind: audit.EvidenceCodeList, MaxItems: 128, MaxLength: 256},
			{Key: "decision.source.role_ids", Kind: audit.EvidenceReferenceList, MaxItems: 128, MaxLength: 512},
			{Key: "decision.source.roles", Kind: audit.EvidenceReferenceList, MaxItems: 128, MaxLength: 512},
			{Key: "decision.source.layers", Kind: audit.EvidenceReferenceList, MaxItems: 128, MaxLength: 512},
			{Key: "decision.source.grant_ids", Kind: audit.EvidenceReferenceList, MaxItems: 128, MaxLength: 512},
			{Key: "decision.source.group_ids", Kind: audit.EvidenceReferenceList, MaxItems: 128, MaxLength: 512},
		},
	})
	return audit.Definition{
		Version: 1, Consumer: "authorization.journal", MaxBatch: 500, MaxEvidence: 32,
		Retention: []audit.RetentionDefinition{
			{Class: authorizationAuditRetention, MinimumAge: 365 * 24 * time.Hour, ArchiveBefore: true},
			{Class: authorizationDecisionRetention, MinimumAge: 90 * 24 * time.Hour},
		},
		Actions: actions,
	}
}

func managementAuditAction(action authorization.AuditAction) audit.Action {
	return audit.Action{Name: audit.ActionName("authorization." + string(action)), Version: 1}
}

func encodeManagementEvidence(value managementEvidence) []audit.EvidenceField {
	fields := []audit.EvidenceField{
		audit.Count("authorization.policy_revision", value.PolicyRevision),
	}
	if value.SubjectKind != "" {
		fields = append(fields, audit.Code("authorization.subject.kind", value.SubjectKind))
	}
	if value.Role != "" {
		fields = append(fields, audit.Code("authorization.role", value.Role))
	}
	if value.Scope != "" {
		fields = append(fields, audit.Reference("authorization.scope", value.Scope))
	}
	return fields
}

func encodeDecisionEvidence(value decisionEvidence) []audit.EvidenceField {
	fields := []audit.EvidenceField{
		audit.Code("decision.subject.kind", value.SubjectKind),
		audit.Code("decision.capability", value.Capability),
		audit.Reference("decision.scope", value.Scope),
		audit.Bool("decision.allowed", value.Allowed),
		audit.Code("decision.reason", value.Reason),
		audit.Count("decision.policy_revision", value.PolicyRevision),
	}
	if value.ResourceType != "" {
		fields = append(fields, audit.Code("decision.resource.type", value.ResourceType))
	}
	if value.ResourceRevision != "" {
		fields = append(fields, audit.Reference("decision.resource.revision", value.ResourceRevision))
	}
	if value.Constraint != "" {
		fields = append(fields, audit.Code("decision.constraint", value.Constraint))
	}
	if len(value.Sources) > 0 {
		kinds := make([]string, len(value.Sources))
		roleIDs := make([]string, len(value.Sources))
		roles := make([]string, len(value.Sources))
		layers := make([]string, len(value.Sources))
		grantIDs := make([]string, len(value.Sources))
		groupIDs := make([]string, len(value.Sources))
		for index, source := range value.Sources {
			kinds[index] = nonemptyReference(source.Kind)
			roleIDs[index] = nonemptyReference(source.RoleID)
			roles[index] = nonemptyReference(source.RoleKey)
			layers[index] = nonemptyReference(source.AccessLayer)
			grantIDs[index] = nonemptyReference(source.GrantID)
			groupIDs[index] = nonemptyReference(source.GroupID)
		}
		fields = append(fields,
			audit.Codes("decision.source.kinds", kinds...),
			audit.References("decision.source.role_ids", roleIDs...),
			audit.References("decision.source.roles", roles...),
			audit.References("decision.source.layers", layers...),
			audit.References("decision.source.grant_ids", grantIDs...),
			audit.References("decision.source.group_ids", groupIDs...),
		)
	}
	return fields
}

func nonemptyReference(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func emptySentinel(value string) string {
	if value == "-" {
		return ""
	}
	return value
}

func (bridge *authorizationAuditBridge) append(
	ctx context.Context,
	tx *sql.Tx,
	management []repository.AuditEvent,
	decisions []repository.DecisionAuditEvent,
) error {
	if len(management) == 0 && len(decisions) == 0 {
		return nil
	}
	appender, err := bridge.journal.Bind(tx)
	if err != nil {
		return err
	}
	commands := make([]audit.Command, 0, len(management)+len(decisions))
	for _, event := range management {
		contract, exists := bridge.management[authorization.AuditAction(event.Action)]
		if !exists {
			return fmt.Errorf("authorization/postgres: unknown management audit action %q", event.Action)
		}
		command, err := audit.Prepare(contract, managementAttempt(event))
		if err != nil {
			return err
		}
		commands = append(commands, command)
	}
	for _, event := range decisions {
		command, err := audit.Prepare(bridge.decision, decisionAttempt(event))
		if err != nil {
			return err
		}
		commands = append(commands, command)
	}
	for len(commands) > 0 {
		size := min(len(commands), 500)
		if _, err := appender.AppendBatch(ctx, commands[:size]); err != nil {
			return err
		}
		commands = commands[size:]
	}
	return nil
}

func (bridge *authorizationAuditBridge) migrateLegacy(ctx context.Context, store stateStore) error {
	var managementTable, decisionTable sql.NullString
	if err := store.db.QueryRowContext(ctx, `
		SELECT to_regclass('authorization_audit_events'),
			to_regclass('authorization_decision_events')
	`).Scan(&managementTable, &decisionTable); err != nil {
		return err
	}
	if !managementTable.Valid && !decisionTable.Valid {
		return nil
	}
	readTx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer func() { _ = readTx.Rollback() }()
	var snapshot repository.Snapshot
	if managementTable.Valid {
		if err := store.loadAudit(ctx, readTx, &snapshot); err != nil {
			return err
		}
	}
	if decisionTable.Valid {
		if err := store.loadDecisionAudit(ctx, readTx, &snapshot); err != nil {
			return err
		}
	}
	if err := readTx.Commit(); err != nil {
		return err
	}
	if len(snapshot.Audit) == 0 && len(snapshot.DecisionAudit) == 0 {
		return nil
	}
	writeTx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer func() { _ = writeTx.Rollback() }()
	if err := bridge.append(ctx, writeTx, snapshot.Audit, snapshot.DecisionAudit); err != nil {
		return err
	}
	return writeTx.Commit()
}

func managementAttempt(event repository.AuditEvent) audit.Attempt[managementEvidence] {
	target := audit.Target{Type: "authorization.event", ID: event.ID}
	if event.Subject.ID != "" {
		target = audit.Target{Type: "authorization.subject", ID: event.Subject.ID}
	} else if event.RoleKey != "" {
		target = audit.Target{Type: "authorization.role", ID: event.RoleKey}
	} else if event.ScopeID != "" {
		target = audit.Target{Type: "authorization.scope", ID: event.ScopeID}
	}
	return audit.Attempt[managementEvidence]{
		ID: audit.EventID(event.ID), Actor: auditActor(event.Actor), Target: target,
		Outcome:     audit.Outcome{Kind: audit.OutcomeSucceeded},
		Correlation: audit.Correlation{RequestID: event.CorrelationID},
		OccurredAt:  event.OccurredAt,
		Evidence: managementEvidence{
			SubjectKind: event.Subject.Kind, Role: event.RoleKey, Scope: event.ScopeID,
			PolicyRevision: event.PolicyRevision,
		},
	}
}

func decisionAttempt(event repository.DecisionAuditEvent) audit.Attempt[decisionEvidence] {
	target := audit.Target{Type: "authorization.decision", ID: event.DecisionID}
	if event.ResourceID != "" {
		target = audit.Target{Type: "authorization.resource", ID: event.ResourceID}
	} else if event.ScopeID != "" {
		target = audit.Target{Type: "authorization.scope", ID: event.ScopeID}
	}
	outcome := audit.Outcome{Kind: audit.OutcomeSucceeded, Reason: audit.ReasonCode(event.Reason)}
	if !event.Allowed {
		outcome.Kind = audit.OutcomeDenied
	}
	return audit.Attempt[decisionEvidence]{
		ID: audit.EventID(event.DecisionID), Actor: auditActor(event.Subject), Target: target,
		Outcome: outcome, Correlation: audit.Correlation{RequestID: event.CorrelationID},
		OccurredAt: event.OccurredAt,
		Evidence: decisionEvidence{
			SubjectKind: event.Subject.Kind, Capability: event.Capability, Scope: event.ScopeID,
			ResourceType: event.ResourceType, ResourceRevision: event.ResourceRevision,
			Allowed: event.Allowed, Reason: event.Reason, Constraint: event.Constraint,
			PolicyRevision: event.PolicyRevision, Sources: slices.Clone(event.Sources),
		},
	}
}

func auditActor(subject repository.Subject) audit.Actor {
	switch authorization.SubjectKind(subject.Kind) {
	case authorization.SubjectAnonymous:
		return audit.Actor{Kind: audit.ActorAnonymous}
	case authorization.SubjectGuest:
		return audit.Actor{Kind: audit.ActorGuest, ID: subject.ID}
	case authorization.SubjectUser:
		return audit.Actor{Kind: audit.ActorUser, ID: subject.ID}
	case authorization.SubjectService:
		return audit.Actor{Kind: audit.ActorService, ID: subject.ID}
	case authorization.SubjectGroup:
		return audit.Actor{Kind: audit.ActorService, ID: "group:" + subject.ID}
	default:
		return audit.Actor{Kind: audit.ActorSystem, ID: "authorization"}
	}
}

func repositorySubject(actor audit.Actor) repository.Subject {
	switch actor.Kind {
	case audit.ActorAnonymous:
		return repository.Subject{Kind: string(authorization.SubjectAnonymous)}
	case audit.ActorGuest:
		return repository.Subject{Kind: string(authorization.SubjectGuest), ID: actor.ID}
	case audit.ActorUser:
		return repository.Subject{Kind: string(authorization.SubjectUser), ID: actor.ID}
	case audit.ActorService:
		if strings.HasPrefix(actor.ID, "group:") {
			return repository.Subject{Kind: string(authorization.SubjectGroup), ID: strings.TrimPrefix(actor.ID, "group:")}
		}
		return repository.Subject{Kind: string(authorization.SubjectService), ID: actor.ID}
	default:
		return repository.Subject{}
	}
}

func (bridge *authorizationAuditBridge) searchManagement(
	ctx context.Context,
	query authorization.AuditQuery,
) (authorization.AuditPage, error) {
	if query.Offset < 0 || query.Limit < 0 {
		return authorization.AuditPage{}, &authorization.Error{Kind: authorization.ErrorInvalidInput, Message: "audit offset and limit cannot be negative"}
	}
	limit := query.Limit
	if limit == 0 {
		limit = 100
	}
	if limit > 500 {
		return authorization.AuditPage{}, &authorization.Error{Kind: authorization.ErrorInvalidInput, Field: "limit", Message: "must not exceed 500"}
	}
	coreQuery := audit.Query{RequestID: query.CorrelationID, Limit: 500}
	if query.Action != "" {
		coreQuery.Actions = []audit.Action{managementAuditAction(query.Action)}
	} else {
		for _, action := range authorizationManagementActions {
			coreQuery.Actions = append(coreQuery.Actions, managementAuditAction(action))
		}
	}
	if query.Actor.Kind != "" {
		actor := auditActor(repository.Subject{Kind: string(query.Actor.Kind), ID: query.Actor.ID})
		coreQuery.Actor = &actor
	}
	events, err := bridge.all(ctx, coreQuery)
	if err != nil {
		return authorization.AuditPage{}, unavailable("query audit journal", err)
	}
	matches := make([]authorization.AuditEvent, 0, len(events))
	for index := len(events) - 1; index >= 0; index-- {
		event, err := decodeManagementAudit(events[index])
		if err != nil {
			return authorization.AuditPage{}, unavailable("decode audit journal", err)
		}
		if query.Subject.Kind != "" && event.Subject != query.Subject ||
			query.Role != "" && event.Role != query.Role ||
			query.ScopeID != "" && event.ScopeID != query.ScopeID {
			continue
		}
		matches = append(matches, event)
	}
	page := authorization.AuditPage{Total: len(matches)}
	if query.Offset < len(matches) {
		page.Events = append([]authorization.AuditEvent(nil), matches[query.Offset:min(query.Offset+limit, len(matches))]...)
	}
	return page, nil
}

func (bridge *authorizationAuditBridge) searchDecisions(
	ctx context.Context,
	query authorization.DecisionAuditQuery,
) (authorization.DecisionAuditPage, error) {
	if query.Offset < 0 || query.Limit < 0 {
		return authorization.DecisionAuditPage{}, &authorization.Error{Kind: authorization.ErrorInvalidInput, Message: "offset and limit must not be negative"}
	}
	limit := query.Limit
	if limit == 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	events, err := bridge.all(ctx, audit.Query{
		Actions:   []audit.Action{{Name: authorizationDecisionAction, Version: 1}},
		RequestID: query.CorrelationID, Limit: 500,
	})
	if err != nil {
		return authorization.DecisionAuditPage{}, unavailable("query decision audit journal", err)
	}
	matches := make([]authorization.DecisionAuditEvent, 0, len(events))
	for index := len(events) - 1; index >= 0; index-- {
		event, err := decodeDecisionAudit(events[index])
		if err != nil {
			return authorization.DecisionAuditPage{}, unavailable("decode decision audit journal", err)
		}
		if query.Subject != (authorization.SubjectRef{}) && event.Subject != query.Subject ||
			query.Capability != "" && event.Capability != query.Capability ||
			query.Allowed != nil && event.Allowed != *query.Allowed {
			continue
		}
		matches = append(matches, event)
	}
	page := authorization.DecisionAuditPage{Total: len(matches)}
	if query.Offset < len(matches) {
		page.Events = append([]authorization.DecisionAuditEvent(nil), matches[query.Offset:min(query.Offset+limit, len(matches))]...)
	}
	return page, nil
}

func (bridge *authorizationAuditBridge) all(ctx context.Context, query audit.Query) ([]audit.Event, error) {
	var events []audit.Event
	for {
		page, err := bridge.journal.Query(ctx, query)
		if err != nil {
			return nil, err
		}
		events = append(events, page.Events...)
		if page.NextCursor == "" {
			return events, nil
		}
		query.Before = page.NextCursor
	}
}

func decodeManagementAudit(event audit.Event) (authorization.AuditEvent, error) {
	actionName := strings.TrimPrefix(string(event.Action.Name), "authorization.")
	fields := evidenceIndex(event.Evidence)
	revision, err := countEvidence(fields, "authorization.policy_revision")
	if err != nil {
		return authorization.AuditEvent{}, err
	}
	subjectKind := textEvidence(fields, "authorization.subject.kind")
	subjectID := ""
	if event.Target.Type == "authorization.subject" {
		subjectID = event.Target.ID
	}
	return authorization.AuditEvent{
		ID: authorization.AuditID(event.ID), Action: authorization.AuditAction(actionName),
		Actor:          subjectRef(repositorySubject(event.Actor)),
		Subject:        authorization.SubjectRef{Kind: authorization.SubjectKind(subjectKind), ID: subjectID},
		Role:           authorization.RoleKey(textEvidence(fields, "authorization.role")),
		ScopeID:        authorization.ScopeID(textEvidence(fields, "authorization.scope")),
		PolicyRevision: revision, CorrelationID: event.Correlation.RequestID,
		OccurredAt: event.OccurredAt,
	}, nil
}

func decodeDecisionAudit(event audit.Event) (authorization.DecisionAuditEvent, error) {
	fields := evidenceIndex(event.Evidence)
	revision, err := countEvidence(fields, "decision.policy_revision")
	if err != nil {
		return authorization.DecisionAuditEvent{}, err
	}
	allowed, err := boolEvidence(fields, "decision.allowed")
	if err != nil {
		return authorization.DecisionAuditEvent{}, err
	}
	subject := subjectRef(repositorySubject(event.Actor))
	subject.Kind = authorization.SubjectKind(textEvidence(fields, "decision.subject.kind"))
	resourceID := ""
	if event.Target.Type == "authorization.resource" {
		resourceID = event.Target.ID
	}
	result := authorization.DecisionAuditEvent{
		DecisionID: authorization.DecisionID(event.ID), Subject: subject,
		Capability:       authorization.CapabilityKey(textEvidence(fields, "decision.capability")),
		ScopeID:          authorization.ScopeID(textEvidence(fields, "decision.scope")),
		ResourceType:     authorization.ResourceType(textEvidence(fields, "decision.resource.type")),
		ResourceID:       authorization.ResourceID(resourceID),
		ResourceRevision: textEvidence(fields, "decision.resource.revision"),
		Allowed:          allowed, Reason: authorization.ReasonCode(textEvidence(fields, "decision.reason")),
		Constraint:     authorization.ConstraintKey(textEvidence(fields, "decision.constraint")),
		PolicyRevision: revision, CorrelationID: event.Correlation.RequestID,
		OccurredAt: event.OccurredAt,
	}
	kinds := listEvidence(fields, "decision.source.kinds")
	roleIDs := listEvidence(fields, "decision.source.role_ids")
	roles := listEvidence(fields, "decision.source.roles")
	layers := listEvidence(fields, "decision.source.layers")
	grantIDs := listEvidence(fields, "decision.source.grant_ids")
	groupIDs := listEvidence(fields, "decision.source.group_ids")
	if len(kinds) > 0 {
		if !equalLengths(kinds, roleIDs, roles, layers, grantIDs, groupIDs) {
			return authorization.DecisionAuditEvent{}, fmt.Errorf("decision source evidence lengths differ")
		}
		for index := range kinds {
			result.Sources = append(result.Sources, authorization.DecisionSource{
				Kind:        authorization.ReasonCode(emptySentinel(kinds[index])),
				RoleID:      authorization.RoleID(emptySentinel(roleIDs[index])),
				Role:        authorization.RoleKey(emptySentinel(roles[index])),
				AccessLayer: authorization.AccessLayerKey(emptySentinel(layers[index])),
				GrantID:     authorization.GrantID(emptySentinel(grantIDs[index])),
				GroupID:     authorization.GroupID(emptySentinel(groupIDs[index])),
			})
		}
	}
	return result, nil
}

func evidenceIndex(fields []audit.EvidenceField) map[audit.EvidenceKey]audit.EvidenceField {
	out := make(map[audit.EvidenceKey]audit.EvidenceField, len(fields))
	for _, field := range fields {
		out[field.Key] = field
	}
	return out
}

func textEvidence(fields map[audit.EvidenceKey]audit.EvidenceField, key audit.EvidenceKey) string {
	return fields[key].Text
}

func listEvidence(fields map[audit.EvidenceKey]audit.EvidenceField, key audit.EvidenceKey) []string {
	return fields[key].List
}

func countEvidence(fields map[audit.EvidenceKey]audit.EvidenceField, key audit.EvidenceKey) (uint64, error) {
	field, found := fields[key]
	if !found || field.Uint == nil {
		return 0, fmt.Errorf("missing count evidence %s", key)
	}
	return *field.Uint, nil
}

func boolEvidence(fields map[audit.EvidenceKey]audit.EvidenceField, key audit.EvidenceKey) (bool, error) {
	field, found := fields[key]
	if !found || field.Bool == nil {
		return false, fmt.Errorf("missing bool evidence %s", key)
	}
	return *field.Bool, nil
}

func equalLengths(first []string, rest ...[]string) bool {
	for _, values := range rest {
		if len(values) != len(first) {
			return false
		}
	}
	return true
}

func subjectRef(subject repository.Subject) authorization.SubjectRef {
	return authorization.SubjectRef{Kind: authorization.SubjectKind(subject.Kind), ID: subject.ID}
}
