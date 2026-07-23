package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

func (adapter *Postgres) PlaceHold(ctx context.Context, command PlaceHoldCommand) (LegalHold, error) {
	if !codePattern.MatchString(command.ID) || !codePattern.MatchString(string(command.Reason)) {
		return LegalHold{}, &Error{Kind: ErrorHoldConflict, Field: "command", Message: "is invalid"}
	}
	if err := validateMaintenanceActor(command.Actor); err != nil {
		return LegalHold{}, err
	}
	selection, err := normalizeHoldSelection(command.Selection)
	if err != nil {
		return LegalHold{}, err
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return LegalHold{}, &Error{Kind: ErrorUnavailable, Field: "db", Message: "begin hold transaction failed"}
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockAuditInstance(ctx, tx, adapter.instanceKey); err != nil {
		return LegalHold{}, err
	}
	existing, found, err := readPostgresHold(ctx, tx, adapter.instanceKey, command.ID)
	if err != nil {
		return LegalHold{}, err
	}
	if found {
		if existing.ReleasedAt == nil && sameHold(existing, command, selection) {
			return existing, nil
		}
		return LegalHold{}, &Error{Kind: ErrorHoldConflict, Field: "id", Message: "was already used"}
	}
	now := adapter.clock.Now().UTC()
	selectionJSON, _ := json.Marshal(selection)
	actorJSON, _ := json.Marshal(command.Actor)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_legal_holds(
			instance_key, id, selection, reason, placed_by, placed_at
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, adapter.instanceKey, command.ID, selectionJSON, command.Reason, actorJSON, now); err != nil {
		return LegalHold{}, &Error{Kind: ErrorUnavailable, Field: "hold", Message: "cannot be placed"}
	}
	if err := tx.Commit(); err != nil {
		return LegalHold{}, &Error{Kind: ErrorUnavailable, Field: "db", Message: "hold commit failed"}
	}
	return LegalHold{
		ID: command.ID, Selection: selection, Reason: command.Reason,
		PlacedBy: command.Actor, PlacedAt: now,
	}, nil
}

func (adapter *Postgres) ReleaseHold(ctx context.Context, command ReleaseHoldCommand) (LegalHold, error) {
	if !codePattern.MatchString(command.ID) || !codePattern.MatchString(string(command.Reason)) {
		return LegalHold{}, &Error{Kind: ErrorHoldConflict, Field: "command", Message: "is invalid"}
	}
	if err := validateMaintenanceActor(command.Actor); err != nil {
		return LegalHold{}, err
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return LegalHold{}, &Error{Kind: ErrorUnavailable, Field: "db", Message: "begin hold transaction failed"}
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockAuditInstance(ctx, tx, adapter.instanceKey); err != nil {
		return LegalHold{}, err
	}
	hold, found, err := readPostgresHold(ctx, tx, adapter.instanceKey, command.ID)
	if err != nil {
		return LegalHold{}, err
	}
	if !found {
		return LegalHold{}, &Error{Kind: ErrorHoldConflict, Field: "id", Message: "does not exist"}
	}
	if hold.ReleasedAt != nil {
		if hold.ReleaseReason == command.Reason && hold.ReleasedBy != nil && *hold.ReleasedBy == command.Actor {
			return hold, nil
		}
		return LegalHold{}, &Error{Kind: ErrorHoldConflict, Field: "id", Message: "was already released"}
	}
	now := adapter.clock.Now().UTC()
	actorJSON, _ := json.Marshal(command.Actor)
	if _, err := tx.ExecContext(ctx, `
		UPDATE audit_legal_holds
		SET release_reason = $3, released_by = $4, released_at = $5
		WHERE instance_key = $1 AND id = $2 AND released_at IS NULL
	`, adapter.instanceKey, command.ID, command.Reason, actorJSON, now); err != nil {
		return LegalHold{}, &Error{Kind: ErrorUnavailable, Field: "hold", Message: "cannot be released"}
	}
	if err := tx.Commit(); err != nil {
		return LegalHold{}, &Error{Kind: ErrorUnavailable, Field: "db", Message: "hold commit failed"}
	}
	actor := command.Actor
	hold.ReleaseReason, hold.ReleasedBy, hold.ReleasedAt = command.Reason, &actor, &now
	return hold, nil
}

func (adapter *Postgres) RunRetention(ctx context.Context, command RetentionCommand) (RetentionReceipt, error) {
	if !codePattern.MatchString(command.ID) {
		return RetentionReceipt{}, invalidAttempt("id", "is invalid")
	}
	if err := validateMaintenanceActor(command.Actor); err != nil {
		return RetentionReceipt{}, err
	}
	if command.AsOf.IsZero() {
		command.AsOf = adapter.clock.Now().UTC()
	} else {
		command.AsOf = command.AsOf.UTC()
	}
	if command.BatchLimit == 0 {
		command.BatchLimit = 1000
	}
	if command.BatchLimit < 1 || command.BatchLimit > 10000 {
		return RetentionReceipt{}, invalidAttempt("batch_limit", "must be between 1 and 10000")
	}
	if receipt, found, err := adapter.readRetentionReceipt(ctx, adapter.db, command.ID); err != nil || found {
		return receipt, err
	}
	var head Sequence
	var headDigest Digest
	if err := adapter.db.QueryRowContext(ctx, `
		SELECT head_sequence, head_digest FROM audit_instances WHERE instance_key = $1
	`, adapter.instanceKey).Scan(&head, &headDigest); err != nil {
		return RetentionReceipt{}, &Error{Kind: ErrorUnavailable, Field: "ledger_head", Message: "cannot be read"}
	}
	candidates, archiveRequired, err := adapter.postgresRetentionCandidates(ctx, adapter.db, command.AsOf, command.BatchLimit)
	if err != nil {
		return RetentionReceipt{}, err
	}
	ranges := sequenceRanges(candidates)
	var archive *ArchiveReceipt
	if len(candidates) > 0 && (archiveRequired || command.Archive != nil) {
		if command.Archive == nil {
			return RetentionReceipt{}, &Error{Kind: ErrorArchiveRequired, Field: "archive", Message: "is required by the retention class"}
		}
		var writtenDigest Digest
		value, err := command.Archive.Put(ctx, ArchiveDescriptor{
			RetentionID: command.ID, Instance: adapter.source.Instance, AsOf: command.AsOf,
			ExpectedCount: uint64(len(candidates)), ExpectedRanges: slices.Clone(ranges),
			DefinitionDigest: adapter.catalog.digest,
		}, func(writer io.Writer) error {
			var writeErr error
			writtenDigest, writeErr = writeRetentionArchive(writer, candidates)
			return writeErr
		})
		if err != nil || value.Reference == "" || value.Count != uint64(len(candidates)) ||
			value.ContentDigest != writtenDigest {
			return RetentionReceipt{}, &Error{Kind: ErrorArchiveRequired, Field: "archive", Message: "receipt does not match written content"}
		}
		archive = &value
	}

	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return RetentionReceipt{}, &Error{Kind: ErrorUnavailable, Field: "db", Message: "begin retention transaction failed"}
	}
	defer func() { _ = tx.Rollback() }()
	var lockedHead Sequence
	var lockedDigest Digest
	if err := tx.QueryRowContext(ctx, `
		SELECT head_sequence, head_digest FROM audit_instances
		WHERE instance_key = $1 FOR UPDATE
	`, adapter.instanceKey).Scan(&lockedHead, &lockedDigest); err != nil {
		return RetentionReceipt{}, &Error{Kind: ErrorUnavailable, Field: "ledger_head", Message: "cannot be locked"}
	}
	if lockedHead != head || lockedDigest != headDigest {
		return RetentionReceipt{}, &Error{Kind: ErrorUnavailable, Field: "journal", Message: "changed while retention archive was written"}
	}
	if receipt, found, err := adapter.readRetentionReceipt(ctx, tx, command.ID); err != nil || found {
		return receipt, err
	}
	confirmed, _, err := adapter.postgresRetentionCandidates(ctx, tx, command.AsOf, command.BatchLimit)
	if err != nil {
		return RetentionReceipt{}, err
	}
	if !sameEventIDs(candidates, confirmed) {
		return RetentionReceipt{}, &Error{Kind: ErrorHoldConflict, Field: "selection", Message: "changed while retention archive was written"}
	}
	now := adapter.clock.Now().UTC()
	for _, event := range candidates {
		if _, err := tx.ExecContext(ctx, `
			UPDATE audit_event_receipts SET purged_at = $3
			WHERE instance_key = $1 AND id = $2 AND purged_at IS NULL
		`, adapter.instanceKey, event.ID, now); err != nil {
			return RetentionReceipt{}, &Error{Kind: ErrorUnavailable, Field: "retention", Message: "cannot preserve idempotency receipt"}
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM audit_events WHERE instance_key = $1 AND id = $2
		`, adapter.instanceKey, event.ID); err != nil {
			return RetentionReceipt{}, &Error{Kind: ErrorUnavailable, Field: "retention", Message: "cannot purge event"}
		}
	}
	rangesJSON, _ := json.Marshal(ranges)
	var archiveJSON []byte
	if archive != nil {
		archiveJSON, _ = json.Marshal(archive)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_retention_receipts(
			instance_key, id, as_of, deleted_count, deleted_ranges, archive_manifest, created_at
		) VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::jsonb, $7)
	`, adapter.instanceKey, command.ID, command.AsOf, len(candidates), rangesJSON, string(archiveJSON), now); err != nil {
		return RetentionReceipt{}, &Error{Kind: ErrorUnavailable, Field: "retention", Message: "cannot store receipt"}
	}
	if err := tx.Commit(); err != nil {
		return RetentionReceipt{}, &Error{Kind: ErrorUnavailable, Field: "db", Message: "retention commit failed"}
	}
	return RetentionReceipt{
		ID: command.ID, AsOf: command.AsOf, Deleted: uint64(len(candidates)),
		Ranges: ranges, Archive: archive, CreatedAt: now,
	}, nil
}

type postgresQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (adapter *Postgres) postgresRetentionCandidates(
	ctx context.Context,
	queryer postgresQueryer,
	asOf time.Time,
	limit int,
) ([]Event, bool, error) {
	holds, err := readActivePostgresHolds(ctx, queryer, adapter.instanceKey)
	if err != nil {
		return nil, false, err
	}
	var where []string
	args := []any{adapter.instanceKey}
	for class, definition := range adapter.catalog.retention {
		args = append(args, class, asOf.Add(-definition.MinimumAge))
		where = append(where, fmt.Sprintf("(retention_class = $%d AND recorded_at <= $%d)", len(args)-1, len(args)))
	}
	statement := eventSelect + " WHERE instance_key = $1 AND (" + strings.Join(where, " OR ") +
		") AND sequence > $" + fmt.Sprint(len(args)+1) + " ORDER BY sequence ASC LIMIT $" + fmt.Sprint(len(args)+2)
	cursor := Sequence(0)
	candidates := make([]Event, 0, limit)
	archiveRequired := false
	for len(candidates) < limit {
		chunk := min(limit*2, 1000)
		rows, err := queryer.QueryContext(ctx, statement, append(args, cursor, chunk)...)
		if err != nil {
			return nil, false, &Error{Kind: ErrorUnavailable, Field: "retention", Message: "candidate query failed"}
		}
		scanned, err := scanPostgresEvents(rows, chunk)
		rows.Close()
		if err != nil {
			return nil, false, err
		}
		if len(scanned) == 0 {
			break
		}
		for _, event := range scanned {
			cursor = event.Sequence
			held := false
			for _, hold := range holds {
				if heldBy(hold.Selection, event) {
					held = true
					break
				}
			}
			if held {
				continue
			}
			candidates = append(candidates, event)
			archiveRequired = archiveRequired || adapter.catalog.retention[event.RetentionClass].ArchiveBefore
			if len(candidates) == limit {
				break
			}
		}
		if len(scanned) < chunk {
			break
		}
	}
	return candidates, archiveRequired, nil
}

func lockAuditInstance(ctx context.Context, tx *sql.Tx, instance string) error {
	var ignored int
	if err := tx.QueryRowContext(ctx, `
		SELECT 1 FROM audit_instances WHERE instance_key = $1 FOR UPDATE
	`, instance).Scan(&ignored); err != nil {
		return &Error{Kind: ErrorUnavailable, Field: "instance", Message: "cannot be locked"}
	}
	return nil
}

func readPostgresHold(
	ctx context.Context,
	queryer postgresQueryer,
	instance string,
	id string,
) (LegalHold, bool, error) {
	return scanPostgresHold(queryer.QueryRowContext(ctx, `
		SELECT id, selection, reason, placed_by, placed_at,
			COALESCE(release_reason, ''), released_by, released_at
		FROM audit_legal_holds WHERE instance_key = $1 AND id = $2
	`, instance, id))
}

func readActivePostgresHolds(
	ctx context.Context,
	queryer postgresQueryer,
	instance string,
) ([]LegalHold, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT id, selection, reason, placed_by, placed_at,
			COALESCE(release_reason, ''), released_by, released_at
		FROM audit_legal_holds
		WHERE instance_key = $1 AND released_at IS NULL
		ORDER BY id
	`, instance)
	if err != nil {
		return nil, &Error{Kind: ErrorUnavailable, Field: "hold", Message: "cannot be queried"}
	}
	defer rows.Close()
	var holds []LegalHold
	for rows.Next() {
		hold, _, err := scanPostgresHold(rows)
		if err != nil {
			return nil, err
		}
		holds = append(holds, hold)
	}
	return holds, nil
}

func scanPostgresHold(row rowScanner) (LegalHold, bool, error) {
	var hold LegalHold
	var selectionJSON, placedByJSON []byte
	var releasedByJSON []byte
	var releasedAt sql.NullTime
	if err := row.Scan(
		&hold.ID, &selectionJSON, &hold.Reason, &placedByJSON, &hold.PlacedAt,
		&hold.ReleaseReason, &releasedByJSON, &releasedAt,
	); err == sql.ErrNoRows {
		return LegalHold{}, false, nil
	} else if err != nil {
		return LegalHold{}, false, &Error{Kind: ErrorUnavailable, Field: "hold", Message: "cannot be scanned"}
	}
	if json.Unmarshal(selectionJSON, &hold.Selection) != nil || json.Unmarshal(placedByJSON, &hold.PlacedBy) != nil {
		return LegalHold{}, false, &Error{Kind: ErrorIntegrityMismatch, Field: "hold", Message: "stored JSON is invalid"}
	}
	if len(releasedByJSON) > 0 {
		var actor Actor
		if json.Unmarshal(releasedByJSON, &actor) != nil {
			return LegalHold{}, false, &Error{Kind: ErrorIntegrityMismatch, Field: "hold", Message: "stored release actor is invalid"}
		}
		hold.ReleasedBy = &actor
	}
	hold.PlacedAt = hold.PlacedAt.UTC()
	if releasedAt.Valid {
		value := releasedAt.Time.UTC()
		hold.ReleasedAt = &value
	}
	return hold, true, nil
}

func (adapter *Postgres) readRetentionReceipt(
	ctx context.Context,
	queryer postgresQueryer,
	id string,
) (RetentionReceipt, bool, error) {
	var receipt RetentionReceipt
	var rangesJSON, archiveJSON []byte
	err := queryer.QueryRowContext(ctx, `
		SELECT id, as_of, deleted_count, deleted_ranges, archive_manifest, created_at
		FROM audit_retention_receipts WHERE instance_key = $1 AND id = $2
	`, adapter.instanceKey, id).Scan(
		&receipt.ID, &receipt.AsOf, &receipt.Deleted, &rangesJSON, &archiveJSON, &receipt.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return RetentionReceipt{}, false, nil
	}
	if err != nil {
		return RetentionReceipt{}, false, &Error{Kind: ErrorUnavailable, Field: "retention", Message: "receipt cannot be read"}
	}
	if json.Unmarshal(rangesJSON, &receipt.Ranges) != nil {
		return RetentionReceipt{}, false, &Error{Kind: ErrorIntegrityMismatch, Field: "retention", Message: "stored ranges are invalid"}
	}
	if len(archiveJSON) > 0 {
		var archive ArchiveReceipt
		if json.Unmarshal(archiveJSON, &archive) != nil {
			return RetentionReceipt{}, false, &Error{Kind: ErrorIntegrityMismatch, Field: "retention", Message: "stored archive receipt is invalid"}
		}
		receipt.Archive = &archive
	}
	receipt.AsOf, receipt.CreatedAt = receipt.AsOf.UTC(), receipt.CreatedAt.UTC()
	return receipt, true, nil
}
