package audit

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

type PostgresOptions struct {
	DB                 *sql.DB
	InstanceKey        string
	Source             Source
	Clock              Clock
	EnableMirrorOutbox bool
}

type Postgres struct {
	db                 *sql.DB
	catalog            *Catalog
	instanceKey        string
	source             Source
	clock              Clock
	enableMirrorOutbox bool
}

type postgresAppender struct {
	adapter *Postgres
	tx      *sql.Tx
}

func NewPostgres(ctx context.Context, catalog *Catalog, options PostgresOptions) (*Postgres, error) {
	if catalog == nil {
		return nil, invalidDefinition("catalog", "is required")
	}
	if options.DB == nil {
		return nil, invalidDefinition("db", "is required")
	}
	options.InstanceKey = strings.TrimSpace(options.InstanceKey)
	if options.InstanceKey == "" || len(options.InstanceKey) > 128 || !codePattern.MatchString(options.InstanceKey) {
		return nil, invalidDefinition("instance_key", "must be a bounded code-like value")
	}
	if options.Source.Service == "" || options.Source.Instance == "" {
		return nil, invalidDefinition("source", "service and instance are required")
	}
	if options.Clock == nil {
		options.Clock = ClockFunc(time.Now)
	}
	if err := options.DB.PingContext(ctx); err != nil {
		return nil, &Error{Kind: ErrorUnavailable, Field: "db", Message: "ping failed"}
	}
	adapter := &Postgres{
		db: options.DB, catalog: catalog, instanceKey: options.InstanceKey,
		source: options.Source, clock: options.Clock, enableMirrorOutbox: options.EnableMirrorOutbox,
	}
	if err := adapter.reconcileDefinition(ctx); err != nil {
		return nil, err
	}
	return adapter, nil
}

func (adapter *Postgres) Bind(tx *sql.Tx) (Appender, error) {
	if adapter == nil || tx == nil {
		return nil, invalidAttempt("transaction", "is required")
	}
	return &postgresAppender{adapter: adapter, tx: tx}, nil
}

func (adapter *Postgres) AppendIndependent(ctx context.Context, command Command) (Event, error) {
	if command.value.Commit != CommitIndependentAllow {
		return Event{}, &Error{Kind: ErrorTransactionRequired, Field: "action", Message: "requires a caller-owned transaction"}
	}
	transaction, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, &Error{Kind: ErrorUnavailable, Field: "db", Message: "begin transaction failed"}
	}
	defer func() { _ = transaction.Rollback() }()
	appender := &postgresAppender{adapter: adapter, tx: transaction}
	event, err := appender.Append(ctx, command)
	if err != nil {
		return Event{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Event{}, &Error{Kind: ErrorUnavailable, Field: "db", Message: "commit failed"}
	}
	return event, nil
}

func (appender *postgresAppender) Append(ctx context.Context, command Command) (Event, error) {
	events, err := appender.AppendBatch(ctx, []Command{command})
	if err != nil {
		return Event{}, err
	}
	return events[0], nil
}

func (appender *postgresAppender) AppendBatch(ctx context.Context, commands []Command) ([]Event, error) {
	adapter := appender.adapter
	if len(commands) == 0 || len(commands) > adapter.catalog.definition.MaxBatch {
		return nil, invalidAttempt("batch", "size is outside the compiled limit")
	}
	seen := make(map[EventID]struct{}, len(commands))
	replays := make([]Event, 0, len(commands))
	replayCount := 0
	for _, command := range commands {
		value := command.value
		if value.ID == "" || value.DefinitionDigest != adapter.catalog.digest {
			return nil, invalidAttempt("command", "was not prepared by this Catalog")
		}
		if _, exists := seen[value.ID]; exists {
			return nil, invalidAttempt("batch", "contains a duplicate event ID")
		}
		seen[value.ID] = struct{}{}
		event, fingerprint, purged, found, err := adapter.getReceiptByIDTx(ctx, appender.tx, value.ID)
		if err != nil {
			return nil, err
		}
		if found {
			if fingerprint != value.Fingerprint {
				return nil, &Error{Kind: ErrorIdempotencyConflict, Field: "id", Message: "was already used for different evidence"}
			}
			if purged {
				return nil, &Error{Kind: ErrorIdempotencyConflict, Field: "id", Message: "belongs to a purged audit event"}
			}
			replayCount++
			replays = append(replays, event)
		}
	}
	if replayCount > 0 {
		if replayCount != len(commands) {
			return nil, &Error{Kind: ErrorIdempotencyConflict, Field: "batch", Message: "is only partially replayed"}
		}
		return replays, nil
	}
	var head uint64
	var previous string
	if err := appender.tx.QueryRowContext(ctx, `
		SELECT head_sequence, head_digest
		FROM audit_instances
		WHERE instance_key = $1
		FOR UPDATE
	`, adapter.instanceKey).Scan(&head, &previous); err != nil {
		return nil, &Error{Kind: ErrorUnavailable, Field: "ledger_head", Message: "cannot be locked"}
	}
	recordedAt := adapter.clock.Now().UTC()
	events := make([]Event, 0, len(commands))
	for index, command := range commands {
		value := command.value
		occurredAt := value.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = recordedAt
		}
		event := Event{
			ID: value.ID, EnvelopeVersion: 1, Sequence: Sequence(head + uint64(index) + 1),
			Source: adapter.source, Action: value.Action, Actor: value.Actor, Target: value.Target,
			Outcome: value.Outcome, Correlation: value.Correlation, Evidence: cloneEvidence(value.Evidence),
			RetentionClass: value.RetentionClass, DefinitionDigest: value.DefinitionDigest,
			OccurredAt: occurredAt.UTC(), RecordedAt: recordedAt, PreviousDigest: Digest(previous),
		}
		digest, err := eventDigest(event)
		if err != nil {
			return nil, &Error{Kind: ErrorUnavailable, Field: "event", Message: "cannot be encoded"}
		}
		event.Digest = digest
		category := adapter.catalog.actions[event.Action].definition.Category
		if err := insertPostgresEvent(ctx, appender.tx, adapter.instanceKey, event, value.Fingerprint, category); err != nil {
			return nil, err
		}
		if adapter.enableMirrorOutbox {
			raw, err := json.Marshal(event)
			if err != nil {
				return nil, &Error{Kind: ErrorUnavailable, Field: "mirror", Message: "event cannot be encoded"}
			}
			if _, err := appender.tx.ExecContext(ctx, `
				INSERT INTO audit_mirror_outbox(instance_key, sequence, event, available_at)
				VALUES ($1, $2, $3, $4)
			`, adapter.instanceKey, event.Sequence, raw, recordedAt); err != nil {
				return nil, &Error{Kind: ErrorUnavailable, Field: "mirror", Message: "outbox append failed"}
			}
		}
		previous = string(digest)
		events = append(events, event)
	}
	if _, err := appender.tx.ExecContext(ctx, `
		UPDATE audit_instances
		SET head_sequence = $2, head_digest = $3, updated_at = $4
		WHERE instance_key = $1
	`, adapter.instanceKey, events[len(events)-1].Sequence, previous, recordedAt); err != nil {
		return nil, &Error{Kind: ErrorUnavailable, Field: "ledger_head", Message: "cannot be advanced"}
	}
	return events, nil
}

func (adapter *Postgres) Query(ctx context.Context, input Query) (Page, error) {
	query, filterDigest, err := normalizeQuery(input)
	if err != nil {
		return Page{}, err
	}
	before, err := decodeCursor(query.Before, filterDigest)
	if err != nil {
		return Page{}, err
	}
	if before == 0 {
		if err := adapter.db.QueryRowContext(ctx, `
			SELECT head_sequence + 1 FROM audit_instances WHERE instance_key = $1
		`, adapter.instanceKey).Scan(&before); err != nil {
			return Page{}, &Error{Kind: ErrorUnavailable, Field: "ledger_head", Message: "cannot be read"}
		}
	}
	statement, args := adapter.querySQL(query, before, query.Limit+1, "DESC")
	rows, err := adapter.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return Page{}, &Error{Kind: ErrorUnavailable, Field: "query", Message: "failed"}
	}
	defer rows.Close()
	events, err := scanPostgresEvents(rows, query.Limit+1)
	if err != nil {
		return Page{}, err
	}
	page := Page{Events: events}
	if len(page.Events) > query.Limit {
		page.Events = page.Events[:query.Limit]
		page.NextCursor = encodeCursor(page.Events[len(page.Events)-1].Sequence, filterDigest)
	}
	return page, nil
}

func (adapter *Postgres) Get(ctx context.Context, id EventID) (Event, bool, error) {
	row := adapter.db.QueryRowContext(ctx, eventSelect+`
		WHERE instance_key = $1 AND id = $2
	`, adapter.instanceKey, id)
	event, _, found, err := scanPostgresEvent(row)
	return event, found, err
}

func (adapter *Postgres) Export(ctx context.Context, request ExportRequest, writer io.Writer) (ExportManifest, error) {
	if writer == nil {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "writer", Message: "is required"}
	}
	if request.Format == "" {
		request.Format = ExportNDJSONV1
	}
	if request.Format != ExportNDJSONV1 || request.Query.Before != "" || request.Query.Limit != 0 {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "request", Message: "is invalid"}
	}
	query := request.Query
	query.Limit = 500
	query, _, err := normalizeQuery(query)
	if err != nil {
		return ExportManifest{}, err
	}
	transaction, err := adapter.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "db", Message: "begin snapshot failed"}
	}
	defer func() { _ = transaction.Rollback() }()
	var head Sequence
	var headDigest Digest
	if err := transaction.QueryRowContext(ctx, `
		SELECT head_sequence, head_digest FROM audit_instances WHERE instance_key = $1
	`, adapter.instanceKey).Scan(&head, &headDigest); err != nil {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "ledger_head", Message: "cannot be read"}
	}
	statement, args := adapter.querySQL(query, head+1, 0, "ASC")
	rows, err := transaction.QueryContext(ctx, statement, args...)
	if err != nil {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "query", Message: "failed"}
	}
	defer rows.Close()
	manifest, err := streamPostgresExport(rows, writer, adapter.catalog.digest, headDigest, adapter.clock.Now().UTC())
	if err != nil {
		return ExportManifest{}, err
	}
	if err := transaction.Commit(); err != nil {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "db", Message: "finish snapshot failed"}
	}
	return manifest, nil
}

func (adapter *Postgres) Verify(ctx context.Context, _ VerifyRequest) (VerifyResult, error) {
	transaction, err := adapter.db.BeginTx(ctx, &sql.TxOptions{
		ReadOnly:  true,
		Isolation: sql.LevelRepeatableRead,
	})
	if err != nil {
		return VerifyResult{}, &Error{Kind: ErrorUnavailable, Field: "db", Message: "begin verify snapshot failed"}
	}
	defer func() { _ = transaction.Rollback() }()
	rows, err := transaction.QueryContext(ctx, eventSelect+`
		WHERE instance_key = $1 ORDER BY sequence ASC
	`, adapter.instanceKey)
	if err != nil {
		return VerifyResult{}, &Error{Kind: ErrorUnavailable, Field: "verify", Message: "query failed"}
	}
	defer rows.Close()
	events, err := scanPostgresEvents(rows, 100)
	if err != nil {
		return VerifyResult{}, err
	}
	rangeRows, err := transaction.QueryContext(ctx, `
		SELECT deleted_ranges FROM audit_retention_receipts
		WHERE instance_key = $1 ORDER BY created_at, id
	`, adapter.instanceKey)
	if err != nil {
		return VerifyResult{}, &Error{Kind: ErrorUnavailable, Field: "verify", Message: "retention receipts cannot be queried"}
	}
	defer rangeRows.Close()
	var ranges []SequenceRange
	for rangeRows.Next() {
		var raw []byte
		var receiptRanges []SequenceRange
		if err := rangeRows.Scan(&raw); err != nil || json.Unmarshal(raw, &receiptRanges) != nil {
			return VerifyResult{}, &Error{Kind: ErrorIntegrityMismatch, Field: "retention", Message: "stored ranges are invalid"}
		}
		ranges = append(ranges, receiptRanges...)
	}
	var head Sequence
	var storedHead Digest
	if err := transaction.QueryRowContext(ctx, `
		SELECT head_sequence, head_digest FROM audit_instances WHERE instance_key = $1
	`, adapter.instanceKey).Scan(&head, &storedHead); err != nil {
		return VerifyResult{}, &Error{Kind: ErrorUnavailable, Field: "ledger_head", Message: "cannot be read"}
	}
	result := verifyJournal(events, ranges, head, storedHead)
	if err := transaction.Commit(); err != nil {
		return VerifyResult{}, &Error{Kind: ErrorUnavailable, Field: "db", Message: "finish verify snapshot failed"}
	}
	return result, nil
}

func (adapter *Postgres) reconcileDefinition(ctx context.Context) error {
	now := adapter.clock.Now().UTC()
	transaction, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return &Error{Kind: ErrorUnavailable, Field: "db", Message: "begin definition transaction failed"}
	}
	defer func() { _ = transaction.Rollback() }()
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO audit_instances(
			instance_key, schema_version, consumer, definition_version, definition_digest,
			head_sequence, head_digest, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 0, '', $6, $6)
		ON CONFLICT (instance_key) DO NOTHING
	`, adapter.instanceKey, PostgresSchemaVersion, adapter.catalog.definition.Consumer,
		adapter.catalog.definition.Version, adapter.catalog.digest, now); err != nil {
		return &Error{Kind: ErrorUnavailable, Field: "instance", Message: "cannot be initialized"}
	}
	var schemaVersion int
	var consumer string
	var definitionVersion uint64
	if err := transaction.QueryRowContext(ctx, `
		SELECT schema_version, consumer, definition_version
		FROM audit_instances WHERE instance_key = $1 FOR UPDATE
	`, adapter.instanceKey).Scan(&schemaVersion, &consumer, &definitionVersion); err != nil {
		return &Error{Kind: ErrorUnavailable, Field: "instance", Message: "cannot be read"}
	}
	if schemaVersion != PostgresSchemaVersion || consumer != adapter.catalog.definition.Consumer ||
		definitionVersion > adapter.catalog.definition.Version {
		return &Error{Kind: ErrorInvalidDefinition, Field: "instance", Message: "stored metadata is incompatible"}
	}
	actions := make([]Action, 0, len(adapter.catalog.actions))
	for action := range adapter.catalog.actions {
		actions = append(actions, action)
	}
	slices.SortFunc(actions, func(a, b Action) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return int(a.Version) - int(b.Version)
	})
	for _, action := range actions {
		compiled := adapter.catalog.actions[action]
		raw, err := json.Marshal(compiled.definition)
		if err != nil {
			return &Error{Kind: ErrorInvalidDefinition, Field: "action", Message: "cannot be encoded"}
		}
		result, err := transaction.ExecContext(ctx, `
			INSERT INTO audit_action_definitions(
				instance_key, action_name, action_version, definition_digest, definition, registered_at
			) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (instance_key, action_name, action_version)
			DO UPDATE SET definition_digest = audit_action_definitions.definition_digest
			WHERE audit_action_definitions.definition_digest = EXCLUDED.definition_digest
		`, adapter.instanceKey, action.Name, action.Version, compiled.digest, raw, now)
		if err != nil {
			return &Error{Kind: ErrorUnavailable, Field: "action", Message: "cannot be registered"}
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return &Error{Kind: ErrorInvalidDefinition, Field: "action", Message: "an existing Action version changed schema"}
		}
	}
	if _, err := transaction.ExecContext(ctx, `
		UPDATE audit_instances
		SET definition_version = $2, definition_digest = $3, updated_at = $4
		WHERE instance_key = $1
	`, adapter.instanceKey, adapter.catalog.definition.Version, adapter.catalog.digest, now); err != nil {
		return &Error{Kind: ErrorUnavailable, Field: "instance", Message: "cannot update definition metadata"}
	}
	if err := transaction.Commit(); err != nil {
		return &Error{Kind: ErrorUnavailable, Field: "db", Message: "definition commit failed"}
	}
	return nil
}

func insertPostgresEvent(
	ctx context.Context,
	tx *sql.Tx,
	instanceKey string,
	event Event,
	fingerprint Digest,
	category Category,
) error {
	source, _ := json.Marshal(event.Source)
	correlation, _ := json.Marshal(event.Correlation)
	evidence, _ := json.Marshal(event.Evidence)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_event_receipts(
			instance_key, id, fingerprint, sequence, event_digest, recorded_at
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, instanceKey, event.ID, fingerprint, event.Sequence, event.Digest, event.RecordedAt); err != nil {
		return &Error{Kind: ErrorUnavailable, Field: "event", Message: "reserve idempotency receipt failed"}
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO audit_events(
			instance_key, id, sequence, envelope_version, source,
			action_name, action_version, category, actor_kind, actor_id,
			target_type, target_id, outcome_kind, outcome_reason, correlation,
			evidence, retention_class, definition_digest, fingerprint,
			occurred_at, recorded_at, previous_digest, digest
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, ''),
			$11, $12, $13, NULLIF($14, ''), $15, $16, $17, $18, $19,
			$20, $21, NULLIF($22, ''), $23
		)
	`, instanceKey, event.ID, event.Sequence, event.EnvelopeVersion, source,
		event.Action.Name, event.Action.Version, category, event.Actor.Kind, event.Actor.ID,
		event.Target.Type, event.Target.ID, event.Outcome.Kind, event.Outcome.Reason, correlation,
		evidence, event.RetentionClass, event.DefinitionDigest, fingerprint,
		event.OccurredAt, event.RecordedAt, event.PreviousDigest, event.Digest)
	if err != nil {
		return &Error{Kind: ErrorUnavailable, Field: "event", Message: "append failed"}
	}
	return nil
}

const eventSelect = `
	SELECT id, sequence, envelope_version, source,
		action_name, action_version, actor_kind, COALESCE(actor_id, ''),
		target_type, target_id, outcome_kind, COALESCE(outcome_reason, ''),
		correlation, evidence, retention_class, definition_digest, fingerprint,
		occurred_at, recorded_at, COALESCE(previous_digest, ''), digest
	FROM audit_events
`

type rowScanner interface {
	Scan(...any) error
}

func scanPostgresEvent(row rowScanner) (Event, Digest, bool, error) {
	var event Event
	var sourceJSON, correlationJSON, evidenceJSON []byte
	var fingerprint Digest
	err := row.Scan(
		&event.ID, &event.Sequence, &event.EnvelopeVersion, &sourceJSON,
		&event.Action.Name, &event.Action.Version, &event.Actor.Kind, &event.Actor.ID,
		&event.Target.Type, &event.Target.ID, &event.Outcome.Kind, &event.Outcome.Reason,
		&correlationJSON, &evidenceJSON, &event.RetentionClass, &event.DefinitionDigest, &fingerprint,
		&event.OccurredAt, &event.RecordedAt, &event.PreviousDigest, &event.Digest,
	)
	if err == sql.ErrNoRows {
		return Event{}, "", false, nil
	}
	if err != nil {
		return Event{}, "", false, &Error{Kind: ErrorUnavailable, Field: "event", Message: "scan failed"}
	}
	if json.Unmarshal(sourceJSON, &event.Source) != nil ||
		json.Unmarshal(correlationJSON, &event.Correlation) != nil ||
		json.Unmarshal(evidenceJSON, &event.Evidence) != nil {
		return Event{}, "", false, &Error{Kind: ErrorIntegrityMismatch, Field: "event", Message: "stored JSON is invalid"}
	}
	event.OccurredAt = event.OccurredAt.UTC()
	event.RecordedAt = event.RecordedAt.UTC()
	return event, fingerprint, true, nil
}

func scanPostgresEvents(rows *sql.Rows, capacity int) ([]Event, error) {
	out := make([]Event, 0, capacity)
	for rows.Next() {
		event, _, found, err := scanPostgresEvent(rows)
		if err != nil {
			return nil, err
		}
		if found {
			out = append(out, event)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, &Error{Kind: ErrorUnavailable, Field: "query", Message: "scan failed"}
	}
	return out, nil
}

func (adapter *Postgres) getByIDTx(ctx context.Context, tx *sql.Tx, id EventID) (Event, Digest, bool, error) {
	return scanPostgresEvent(tx.QueryRowContext(ctx, eventSelect+`
		WHERE instance_key = $1 AND id = $2
	`, adapter.instanceKey, id))
}

func (adapter *Postgres) getReceiptByIDTx(
	ctx context.Context,
	tx *sql.Tx,
	id EventID,
) (Event, Digest, bool, bool, error) {
	var fingerprint Digest
	var purgedAt sql.NullTime
	err := tx.QueryRowContext(ctx, `
		SELECT fingerprint, purged_at
		FROM audit_event_receipts
		WHERE instance_key = $1 AND id = $2
	`, adapter.instanceKey, id).Scan(&fingerprint, &purgedAt)
	if err == sql.ErrNoRows {
		return Event{}, "", false, false, nil
	}
	if err != nil {
		return Event{}, "", false, false, &Error{Kind: ErrorUnavailable, Field: "event", Message: "idempotency receipt cannot be read"}
	}
	if purgedAt.Valid {
		return Event{}, fingerprint, true, true, nil
	}
	event, _, found, err := adapter.getByIDTx(ctx, tx, id)
	if err != nil {
		return Event{}, "", false, false, err
	}
	if !found {
		return Event{}, "", false, false, &Error{Kind: ErrorIntegrityMismatch, Field: "event", Message: "receipt references a missing event"}
	}
	return event, fingerprint, false, true, nil
}

func (adapter *Postgres) querySQL(query Query, before Sequence, limit int, order string) (string, []any) {
	var statement strings.Builder
	statement.WriteString(eventSelect)
	statement.WriteString(" WHERE instance_key = $1 AND sequence < $2")
	args := []any{adapter.instanceKey, before}
	add := func(sqlFragment string, value any) {
		args = append(args, value)
		statement.WriteString(fmt.Sprintf(sqlFragment, len(args)))
	}
	if len(query.Actions) > 0 {
		parts := make([]string, 0, len(query.Actions))
		for _, action := range query.Actions {
			args = append(args, action.Name, action.Version)
			parts = append(parts, fmt.Sprintf("(action_name = $%d AND action_version = $%d)", len(args)-1, len(args)))
		}
		statement.WriteString(" AND (" + strings.Join(parts, " OR ") + ")")
	}
	if query.Actor != nil {
		add(" AND actor_kind = $%d", query.Actor.Kind)
		add(" AND COALESCE(actor_id, '') = $%d", query.Actor.ID)
	}
	if query.Target != nil {
		add(" AND target_type = $%d", query.Target.Type)
		add(" AND target_id = $%d", query.Target.ID)
	}
	if len(query.Outcomes) > 0 {
		placeholders := make([]string, 0, len(query.Outcomes))
		for _, outcome := range query.Outcomes {
			args = append(args, outcome)
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
		}
		statement.WriteString(" AND outcome_kind IN (" + strings.Join(placeholders, ",") + ")")
	}
	if len(query.RetentionClasses) > 0 {
		placeholders := make([]string, 0, len(query.RetentionClasses))
		for _, class := range query.RetentionClasses {
			args = append(args, class)
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
		}
		statement.WriteString(" AND retention_class IN (" + strings.Join(placeholders, ",") + ")")
	}
	if query.RequestID != "" {
		add(" AND correlation->>'requestId' = $%d", query.RequestID)
	}
	if query.TraceID != "" {
		add(" AND correlation->>'traceId' = $%d", query.TraceID)
	}
	if query.CommandID != "" {
		add(" AND correlation->>'commandId' = $%d", query.CommandID)
	}
	if !query.From.IsZero() {
		add(" AND occurred_at >= $%d", query.From)
	}
	if !query.To.IsZero() {
		add(" AND occurred_at < $%d", query.To)
	}
	if order != "ASC" {
		order = "DESC"
	}
	statement.WriteString(" ORDER BY sequence " + order)
	if limit > 0 {
		args = append(args, limit)
		statement.WriteString(fmt.Sprintf(" LIMIT $%d", len(args)))
	}
	return statement.String(), args
}

func streamPostgresExport(
	rows *sql.Rows,
	writer io.Writer,
	definitionDigest Digest,
	headDigest Digest,
	generatedAt time.Time,
) (ExportManifest, error) {
	hash := sha256.New()
	buffered := bufio.NewWriter(io.MultiWriter(writer, hash))
	encoder := json.NewEncoder(buffered)
	if err := encoder.Encode(map[string]any{
		"kind": "audit.export.header", "version": 1,
		"definitionDigest": definitionDigest, "generatedAt": generatedAt,
	}); err != nil {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "writer", Message: "write failed"}
	}
	manifest := ExportManifest{
		EnvelopeVersion: 1, DefinitionDigest: definitionDigest,
		GeneratedAt: generatedAt, HeadDigest: headDigest,
	}
	for rows.Next() {
		event, _, found, err := scanPostgresEvent(rows)
		if err != nil {
			return ExportManifest{}, err
		}
		if !found {
			continue
		}
		if manifest.Count == 0 {
			manifest.FirstSequence = event.Sequence
		}
		manifest.Count++
		manifest.LastSequence = event.Sequence
		if err := encoder.Encode(struct {
			Kind  string `json:"kind"`
			Event Event  `json:"event"`
		}{"audit.event", event}); err != nil {
			return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "writer", Message: "write failed"}
		}
	}
	if err := rows.Err(); err != nil {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "query", Message: "scan failed"}
	}
	if err := encoder.Encode(map[string]any{
		"kind": "audit.export.footer", "count": manifest.Count,
		"firstSequence": manifest.FirstSequence, "lastSequence": manifest.LastSequence,
		"headDigest": manifest.HeadDigest,
	}); err != nil {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "writer", Message: "write failed"}
	}
	if err := buffered.Flush(); err != nil {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "writer", Message: "flush failed"}
	}
	manifest.ContentDigest = Digest(hex.EncodeToString(hash.Sum(nil)))
	return manifest, nil
}
