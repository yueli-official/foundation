package privacy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

func (runtime *PostgresRuntime) Track(ctx context.Context, command RetentionCommand) (RetentionItem, error) {
	validator := &Memory{catalog: runtime.catalog, clock: runtime.clock}
	if err := validator.validateIdempotency(command.IdempotencyKey); err != nil {
		return RetentionItem{}, err
	}
	if strings.TrimSpace(command.Record.Value) == "" || len(command.Record.Value) > runtime.catalog.limits.MaxReferenceBytes ||
		!validKey(string(command.Record.Dataset)) || command.TriggeredAt.IsZero() {
		return RetentionItem{}, invalid("retention", "record and trigger are required")
	}
	rule, exists := runtime.catalog.retention[command.Rule]
	if !exists {
		return RetentionItem{}, &Error{Kind: ErrorNotFound, Field: "rule", Message: "is not defined"}
	}
	command.TriggeredAt = command.TriggeredAt.UTC()
	fingerprintValue := fingerprint(command)
	var result RetentionItem
	err := runtime.mutate(ctx, func(queryer privacyQueryer) error {
		existing, found, err := loadPostgresCommand[RetentionItem](ctx, queryer, runtime.instanceKey, command.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if existing.kind != "retention_track" || existing.fingerprint != fingerprintValue {
				return conflict("idempotency_key", "is reused with a different command")
			}
			result = existing.receipt
			result.Replay = true
			return nil
		}
		result = RetentionItem{
			ID: RetentionItemID(receiptID("retention", fingerprintValue)), Record: command.Record, Rule: command.Rule,
			TriggeredAt: command.TriggeredAt, ReviewAt: rule.ReviewAfter.Add(command.TriggeredAt).UTC(),
			State: RetentionTracked, Fingerprint: fingerprintValue,
		}
		if !result.ReviewAt.After(runtime.clock().UTC()) {
			result.State = RetentionDue
		}
		return runtime.insertRetentionCommand(ctx, queryer, command.IdempotencyKey, "retention_track", fingerprintValue, result)
	})
	return result, err
}

func (runtime *PostgresRuntime) Review(ctx context.Context, command RetentionReviewCommand) (RetentionItem, error) {
	validator := &Memory{catalog: runtime.catalog, clock: runtime.clock}
	if err := validator.validateIdempotency(command.IdempotencyKey); err != nil {
		return RetentionItem{}, err
	}
	if command.ItemID == "" || !validDisposition(command.Outcome) {
		return RetentionItem{}, invalid("review", "item and valid outcome are required")
	}
	if (command.Outcome == DispositionRetained || command.Outcome == DispositionRefused) && command.Reason == "" {
		return RetentionItem{}, invalid("reason", "is required for retained or refused outcomes")
	}
	if command.Outcome == DispositionRetained && command.ReviewAfter == nil && strings.TrimSpace(command.HoldRef) == "" {
		return RetentionItem{}, invalid("review_after", "is required for retained outcomes without a hold")
	}
	fingerprintValue := fingerprint(command)
	var result RetentionItem
	err := runtime.mutate(ctx, func(queryer privacyQueryer) error {
		existing, found, err := loadPostgresCommand[RetentionItem](ctx, queryer, runtime.instanceKey, command.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if existing.kind != "retention_review" || existing.fingerprint != fingerprintValue {
				return conflict("idempotency_key", "is reused with a different command")
			}
			result = existing.receipt
			result.Replay = true
			return nil
		}
		var encoded []byte
		err = queryer.QueryRowContext(ctx, `
SELECT item FROM privacy_retention_items WHERE instance_key=$1 AND item_id=$2 FOR UPDATE`,
			runtime.instanceKey, command.ItemID,
		).Scan(&encoded)
		if errors.Is(err, sql.ErrNoRows) {
			return &Error{Kind: ErrorNotFound, Field: "item_id", Message: "is not found"}
		}
		if err != nil {
			return postgresError("load retention item", err)
		}
		if err := json.Unmarshal(encoded, &result); err != nil {
			return postgresError("decode retention item", err)
		}
		result.LastOutcome, result.Reason = command.Outcome, command.Reason
		if command.Outcome == DispositionRetained {
			result.State = RetentionRetained
			if command.ReviewAfter != nil {
				result.ReviewAt = command.ReviewAfter.UTC()
			}
		} else {
			result.State = RetentionCompleted
		}
		receiptJSON, _ := json.Marshal(result)
		if _, err := queryer.ExecContext(ctx, `
INSERT INTO privacy_command_receipts(
    instance_key, idempotency_key, command_kind, fingerprint, receipt, recorded_at
) VALUES ($1,$2,'retention_review',$3,$4,$5)`,
			runtime.instanceKey, command.IdempotencyKey, fingerprintValue, receiptJSON, runtime.clock().UTC(),
		); err != nil {
			return postgresError("insert retention review receipt", err)
		}
		if _, err := queryer.ExecContext(ctx, `
UPDATE privacy_retention_items
SET review_at=$3, state=$4, item=$5, updated_at=$6
WHERE instance_key=$1 AND item_id=$2`,
			runtime.instanceKey, result.ID, result.ReviewAt, result.State, receiptJSON, runtime.clock().UTC(),
		); err != nil {
			return postgresError("update retention item", err)
		}
		return nil
	})
	return result, err
}

func (runtime *PostgresRuntime) Due(ctx context.Context, query RetentionDueQuery) (RetentionPage, error) {
	at := query.At
	if at.IsZero() {
		at = runtime.clock()
	}
	limit := query.Limit
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > runtime.catalog.limits.MaxDuePage {
		return RetentionPage{}, invalid("limit", "is out of range")
	}
	var page RetentionPage
	err := runtime.mutate(ctx, func(queryer privacyQueryer) error {
		rows, err := queryer.QueryContext(ctx, `
SELECT item_id, item
FROM privacy_retention_items
WHERE instance_key=$1 AND state IN ('tracked','due','retained')
  AND review_at <= $2 AND item_id > $3
ORDER BY item_id
LIMIT $4`,
			runtime.instanceKey, at, query.Cursor, limit+1,
		)
		if err != nil {
			return postgresError("query due retention", err)
		}
		var updates []RetentionItem
		for rows.Next() {
			var id RetentionItemID
			var encoded []byte
			if err := rows.Scan(&id, &encoded); err != nil {
				return postgresError("scan due retention", err)
			}
			var item RetentionItem
			if err := json.Unmarshal(encoded, &item); err != nil {
				return postgresError("decode due retention", err)
			}
			item.State = RetentionDue
			if len(page.Items) < limit {
				page.Items = append(page.Items, item)
				updates = append(updates, item)
			} else {
				page.NextCursor = string(page.Items[len(page.Items)-1].ID)
			}
		}
		if err := rows.Close(); err != nil {
			return postgresError("close due retention", err)
		}
		if err := rows.Err(); err != nil {
			return postgresError("iterate due retention", err)
		}
		for _, item := range updates {
			updated, _ := json.Marshal(item)
			if _, err := queryer.ExecContext(ctx, `
UPDATE privacy_retention_items SET state='due', item=$3, updated_at=$4
WHERE instance_key=$1 AND item_id=$2`,
				runtime.instanceKey, item.ID, updated, runtime.clock().UTC(),
			); err != nil {
				return postgresError("mark retention due", err)
			}
		}
		return nil
	})
	return page, err
}

func (runtime *PostgresRuntime) insertRetentionCommand(
	ctx context.Context,
	queryer privacyQueryer,
	key IdempotencyKey,
	kind, fingerprintValue string,
	item RetentionItem,
) error {
	encoded, _ := json.Marshal(item)
	now := runtime.clock().UTC()
	if _, err := queryer.ExecContext(ctx, `
INSERT INTO privacy_command_receipts(
    instance_key, idempotency_key, command_kind, fingerprint, receipt, recorded_at
) VALUES ($1,$2,$3,$4,$5,$6)`,
		runtime.instanceKey, key, kind, fingerprintValue, encoded, now,
	); err != nil {
		return postgresError("insert retention command receipt", err)
	}
	if _, err := queryer.ExecContext(ctx, `
INSERT INTO privacy_retention_items(
    instance_key, item_id, record_dataset, record_value, rule_key, rule_revision,
    review_at, state, item, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		runtime.instanceKey, item.ID, item.Record.Dataset, item.Record.Value,
		item.Rule.Key, item.Rule.Revision, item.ReviewAt, item.State, encoded, now,
	); err != nil {
		return postgresError("insert retention item", err)
	}
	return nil
}
