package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type mirrorLease struct {
	sequence Sequence
	event    Event
}

func (adapter *Postgres) DispatchMirror(
	ctx context.Context,
	mirror Mirror,
	options MirrorDispatchOptions,
) (MirrorDispatchResult, error) {
	if mirror == nil {
		return MirrorDispatchResult{}, invalidAttempt("mirror", "is required")
	}
	if !adapter.enableMirrorOutbox {
		return MirrorDispatchResult{}, invalidAttempt("mirror", "outbox is disabled")
	}
	if options.BatchSize == 0 {
		options.BatchSize = 100
	}
	if options.BatchSize < 1 || options.BatchSize > 1000 {
		return MirrorDispatchResult{}, invalidAttempt("batch_size", "must be between 1 and 1000")
	}
	if options.Lease <= 0 {
		options.Lease = time.Minute
	}
	if options.Retry <= 0 {
		options.Retry = 30 * time.Second
	}
	if options.Clock == nil {
		options.Clock = adapter.clock
	}
	now := options.Clock.Now().UTC()
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return MirrorDispatchResult{}, &Error{Kind: ErrorUnavailable, Field: "mirror", Message: "begin lease transaction failed"}
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
		SELECT sequence, event
		FROM audit_mirror_outbox
		WHERE instance_key = $1 AND delivered_at IS NULL AND available_at <= $2
		ORDER BY sequence
		LIMIT $3
		FOR UPDATE SKIP LOCKED
	`, adapter.instanceKey, now, options.BatchSize)
	if err != nil {
		return MirrorDispatchResult{}, &Error{Kind: ErrorUnavailable, Field: "mirror", Message: "outbox cannot be leased"}
	}
	var leased []mirrorLease
	for rows.Next() {
		var item mirrorLease
		var raw []byte
		if err := rows.Scan(&item.sequence, &raw); err != nil || json.Unmarshal(raw, &item.event) != nil {
			rows.Close()
			return MirrorDispatchResult{}, &Error{Kind: ErrorIntegrityMismatch, Field: "mirror", Message: "outbox event is invalid"}
		}
		leased = append(leased, item)
	}
	rows.Close()
	for _, item := range leased {
		if _, err := tx.ExecContext(ctx, `
			UPDATE audit_mirror_outbox
			SET attempts = attempts + 1, available_at = $3
			WHERE instance_key = $1 AND sequence = $2
		`, adapter.instanceKey, item.sequence, now.Add(options.Lease)); err != nil {
			return MirrorDispatchResult{}, &Error{Kind: ErrorUnavailable, Field: "mirror", Message: "outbox lease cannot be stored"}
		}
	}
	if err := tx.Commit(); err != nil {
		return MirrorDispatchResult{}, &Error{Kind: ErrorUnavailable, Field: "mirror", Message: "outbox lease commit failed"}
	}
	result := MirrorDispatchResult{Selected: len(leased)}
	if len(leased) == 0 {
		return result, nil
	}
	committed := make([]CommittedEvent, len(leased))
	for index, item := range leased {
		committed[index] = CommittedEvent{Event: item.event, CommittedAt: now}
	}
	if err := mirror.Publish(ctx, committed); err != nil {
		result.Failed = len(leased)
		if updateErr := adapter.finishMirrorLease(ctx, leased, now.Add(options.Retry), false); updateErr != nil {
			return result, updateErr
		}
		return result, &Error{Kind: ErrorUnavailable, Field: "mirror", Message: "publish failed"}
	}
	if err := adapter.finishMirrorLease(ctx, leased, now, true); err != nil {
		return result, err
	}
	result.Delivered = len(leased)
	return result, nil
}

func (adapter *Postgres) finishMirrorLease(
	ctx context.Context,
	leased []mirrorLease,
	at time.Time,
	delivered bool,
) error {
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return &Error{Kind: ErrorUnavailable, Field: "mirror", Message: "begin completion transaction failed"}
	}
	defer func() { _ = tx.Rollback() }()
	for _, item := range leased {
		if delivered {
			_, err = tx.ExecContext(ctx, `
				UPDATE audit_mirror_outbox
				SET delivered_at = $3, available_at = $3, last_error_code = NULL
				WHERE instance_key = $1 AND sequence = $2 AND delivered_at IS NULL
			`, adapter.instanceKey, item.sequence, at)
		} else {
			_, err = tx.ExecContext(ctx, `
				UPDATE audit_mirror_outbox
				SET available_at = $3, last_error_code = 'publish_failed'
				WHERE instance_key = $1 AND sequence = $2 AND delivered_at IS NULL
			`, adapter.instanceKey, item.sequence, at)
		}
		if err != nil {
			return &Error{Kind: ErrorUnavailable, Field: "mirror", Message: "outbox completion cannot be stored"}
		}
	}
	if err := tx.Commit(); err != nil {
		return &Error{Kind: ErrorUnavailable, Field: "mirror", Message: "outbox completion commit failed"}
	}
	return nil
}

func (adapter *Postgres) MirrorBacklog(ctx context.Context) (MirrorBacklog, error) {
	var result MirrorBacklog
	var oldest sql.NullTime
	if err := adapter.db.QueryRowContext(ctx, `
		SELECT COUNT(*), MIN(available_at)
		FROM audit_mirror_outbox
		WHERE instance_key = $1 AND delivered_at IS NULL
	`, adapter.instanceKey).Scan(&result.Pending, &oldest); err != nil {
		return MirrorBacklog{}, &Error{Kind: ErrorUnavailable, Field: "mirror", Message: "backlog cannot be read"}
	}
	if oldest.Valid {
		value := oldest.Time.UTC()
		result.OldestPending = &value
	}
	return result, nil
}
