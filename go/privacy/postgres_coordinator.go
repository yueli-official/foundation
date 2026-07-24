package privacy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type PostgresCoordinator struct {
	db          *sql.DB
	instanceKey string
	catalog     *Catalog
	clock       func() time.Time
	router      OwnerRouter
}

var _ Coordinator = (*PostgresCoordinator)(nil)

func NewPostgresCoordinator(
	ctx context.Context,
	catalog *Catalog,
	options PostgresOptions,
	router OwnerRouter,
) (*PostgresCoordinator, error) {
	if router == nil {
		return nil, invalid("router", "is required")
	}
	runtime, err := NewPostgresRuntime(ctx, catalog, options)
	if err != nil {
		return nil, err
	}
	if len(catalog.owners) == 0 || len(catalog.rights) == 0 {
		return nil, invalid("catalog.coordination", "must declare owners and rights policies")
	}
	return &PostgresCoordinator{
		db: runtime.db, instanceKey: runtime.instanceKey, catalog: catalog,
		clock: runtime.clock, router: router,
	}, nil
}

func (coordinator *PostgresCoordinator) Open(ctx context.Context, command OpenRightsRequest) (RightsRequestView, error) {
	now := coordinator.clock().UTC()
	validator := &MemoryCoordinator{catalog: coordinator.catalog, clock: coordinator.clock}
	prepared, policy, owners, err := validator.prepareOpen(command, now)
	if err != nil {
		return RightsRequestView{}, err
	}
	fingerprintValue := fingerprint(prepared)
	tx, err := coordinator.db.BeginTx(ctx, nil)
	if err != nil {
		return RightsRequestView{}, postgresError("begin rights request", err)
	}
	defer func() { _ = tx.Rollback() }()
	var existingID RightsRequestID
	var existingFingerprint string
	err = tx.QueryRowContext(ctx, `
SELECT request_id, fingerprint
FROM privacy_rights_requests
WHERE instance_key=$1 AND idempotency_key=$2
FOR UPDATE`, coordinator.instanceKey, prepared.IdempotencyKey).Scan(&existingID, &existingFingerprint)
	if err == nil {
		if existingFingerprint != fingerprintValue {
			return RightsRequestView{}, conflict("idempotency_key", "is reused with a different request")
		}
		view, err := coordinator.getWith(ctx, tx, existingID)
		if err != nil {
			return RightsRequestView{}, err
		}
		view.Replay = true
		if err := tx.Commit(); err != nil {
			return RightsRequestView{}, postgresError("commit request replay", err)
		}
		return view, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RightsRequestView{}, postgresError("load rights request replay", err)
	}
	id := RightsRequestID(receiptID("request", fingerprintValue))
	deadline := policy.RespondWithin.Add(prepared.RequestedAt).UTC()
	subjectJSON, _ := json.Marshal(prepared.Subject)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO privacy_rights_requests(
    instance_key, request_id, idempotency_key, fingerprint, operation, subject,
    phase, deadline, requested_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,'open',$7,$8,$9)`,
		coordinator.instanceKey, id, prepared.IdempotencyKey, fingerprintValue,
		prepared.Operation, subjectJSON, deadline, prepared.RequestedAt, now,
	); err != nil {
		return RightsRequestView{}, postgresError("insert rights request", err)
	}
	for _, owner := range owners {
		taskID := OwnerTaskID(receiptID("task", fingerprint(struct {
			Request   RightsRequestID
			Owner     OwnerRef
			Operation RightsOperation
		}{id, owner.Ref, prepared.Operation})))
		ownerCommand := OwnerCommand{
			ProtocolVersion: OwnerProtocolVersion, RequestID: id, TaskID: taskID,
			Owner: owner.Ref, Operation: prepared.Operation, Subject: prepared.Subject,
			Datasets: datasetsForOperation(owner, prepared.Operation), RequestedAt: prepared.RequestedAt,
			Deadline: deadline,
		}
		ownerCommand.Fingerprint = commandFingerprintForOwner(ownerCommand)
		encoded, _ := json.Marshal(ownerCommand)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO privacy_owner_tasks(
    instance_key, request_id, task_id, owner_key, phase, command_fingerprint, command, updated_at
) VALUES ($1,$2,$3,$4,'pending',$5,$6,$7)`,
			coordinator.instanceKey, id, taskID, owner.Ref.Key, ownerCommand.Fingerprint, encoded, now,
		); err != nil {
			return RightsRequestView{}, postgresError("insert owner task", err)
		}
	}
	view, err := coordinator.getWith(ctx, tx, id)
	if err != nil {
		return RightsRequestView{}, err
	}
	if err := tx.Commit(); err != nil {
		return RightsRequestView{}, postgresError("commit rights request", err)
	}
	return view, nil
}

func (coordinator *PostgresCoordinator) Get(ctx context.Context, id RightsRequestID) (RightsRequestView, error) {
	return coordinator.getWith(ctx, coordinator.db, id)
}

func (coordinator *PostgresCoordinator) getWith(ctx context.Context, queryer privacyQueryer, id RightsRequestID) (RightsRequestView, error) {
	var view RightsRequestView
	err := queryer.QueryRowContext(ctx, `
SELECT request_id, operation, phase, deadline, requested_at, updated_at, fingerprint
FROM privacy_rights_requests WHERE instance_key=$1 AND request_id=$2`,
		coordinator.instanceKey, id,
	).Scan(&view.ID, &view.Operation, &view.Phase, &view.Deadline, &view.RequestedAt, &view.UpdatedAt, &view.Fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return RightsRequestView{}, &Error{Kind: ErrorNotFound, Field: "request", Message: "is not found"}
	}
	if err != nil {
		return RightsRequestView{}, postgresError("load rights request", err)
	}
	rows, err := queryer.QueryContext(ctx, `
SELECT task_id, owner_key, phase, attempt, next_attempt_at, command, terminal_receipt
FROM privacy_owner_tasks
WHERE instance_key=$1 AND request_id=$2
ORDER BY task_id`, coordinator.instanceKey, id)
	if err != nil {
		return RightsRequestView{}, postgresError("query owner tasks", err)
	}
	defer rows.Close()
	var summary RightsSummary
	for rows.Next() {
		var task OwnerTaskView
		var ownerKey OwnerKey
		var next sql.NullTime
		var commandJSON []byte
		var receiptJSON []byte
		if err := rows.Scan(&task.ID, &ownerKey, &task.Phase, &task.Attempt, &next, &commandJSON, &receiptJSON); err != nil {
			return RightsRequestView{}, postgresError("scan owner task", err)
		}
		var command OwnerCommand
		if err := json.Unmarshal(commandJSON, &command); err != nil {
			return RightsRequestView{}, postgresError("decode owner command", err)
		}
		task.Owner = command.Owner
		if next.Valid {
			value := next.Time.UTC()
			task.NextAttemptAt = &value
		}
		if len(receiptJSON) > 0 {
			var receipt OwnerReceipt
			if err := json.Unmarshal(receiptJSON, &receipt); err != nil {
				return RightsRequestView{}, postgresError("decode owner receipt", err)
			}
			task.Receipt = &receipt
		}
		if task.Phase != TaskTerminal || task.Receipt == nil {
			summary.Pending++
		} else {
			for _, result := range task.Receipt.Results {
				switch result.Disposition {
				case DispositionRetained:
					summary.Retained++
				case DispositionRefused:
					summary.Refused++
				case DispositionNotFound:
					summary.NoRecords++
				default:
					summary.Performed++
				}
			}
		}
		view.Tasks = append(view.Tasks, task)
		_ = ownerKey
	}
	if err := rows.Err(); err != nil {
		return RightsRequestView{}, postgresError("iterate owner tasks", err)
	}
	terminal := 0
	for _, task := range view.Tasks {
		if task.Phase == TaskTerminal {
			terminal++
		}
	}
	switch {
	case terminal == len(view.Tasks):
		view.Phase = RequestComplete
	case terminal > 0:
		view.Phase = RequestPartial
	case view.Phase != RequestOpen:
		view.Phase = RequestActive
	}
	view.Overdue = coordinator.clock().UTC().After(view.Deadline) && view.Phase != RequestComplete
	view.Summary = summary
	return view, nil
}

func (coordinator *PostgresCoordinator) Drive(ctx context.Context, command DriveRightsRequest) (DriveResult, error) {
	budget := command.Budget
	if budget.MaxOwnerAttempts == 0 {
		budget.MaxOwnerAttempts = coordinator.catalog.limits.MaxDriveAttempts
	}
	if budget.MaxDuration == 0 {
		budget.MaxDuration = coordinator.catalog.limits.MaxDriveDuration
	}
	if budget.MaxOwnerAttempts < 1 || budget.MaxOwnerAttempts > coordinator.catalog.limits.MaxDriveAttempts ||
		budget.MaxDuration <= 0 || budget.MaxDuration > coordinator.catalog.limits.MaxDriveDuration {
		return DriveResult{}, invalid("budget", "is out of range")
	}
	stopAt := coordinator.clock().Add(budget.MaxDuration)
	for attempt := 0; attempt < budget.MaxOwnerAttempts && coordinator.clock().Before(stopAt); attempt++ {
		task, found, err := coordinator.claim(ctx, command.Request)
		if err != nil {
			return DriveResult{}, err
		}
		if !found {
			break
		}
		host, err := coordinator.router.Owner(ctx, task.Owner.Key)
		if err != nil {
			if updateErr := coordinator.retry(ctx, task.TaskID, nil); updateErr != nil {
				return DriveResult{}, updateErr
			}
			continue
		}
		receipt, err := host.Handle(ctx, task)
		if err != nil {
			if updateErr := coordinator.retry(ctx, task.TaskID, nil); updateErr != nil {
				return DriveResult{}, updateErr
			}
			continue
		}
		if err := validateOwnerReceipt(task, receipt); err != nil {
			_ = coordinator.retry(ctx, task.TaskID, nil)
			return DriveResult{}, err
		}
		if err := coordinator.accept(ctx, task, receipt); err != nil {
			return DriveResult{}, err
		}
	}
	view, err := coordinator.Get(ctx, command.Request)
	if err != nil {
		return DriveResult{}, err
	}
	var next *time.Time
	for _, task := range view.Tasks {
		if task.NextAttemptAt != nil && task.Phase != TaskTerminal && (next == nil || task.NextAttemptAt.Before(*next)) {
			value := *task.NextAttemptAt
			next = &value
		}
	}
	return DriveResult{View: view, NextAttemptAt: next}, nil
}

func (coordinator *PostgresCoordinator) claim(ctx context.Context, request RightsRequestID) (OwnerCommand, bool, error) {
	tx, err := coordinator.db.BeginTx(ctx, nil)
	if err != nil {
		return OwnerCommand{}, false, postgresError("begin task claim", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := coordinator.clock().UTC()
	var locked RightsRequestID
	err = tx.QueryRowContext(ctx, `
SELECT request_id FROM privacy_rights_requests
WHERE instance_key=$1 AND request_id=$2
FOR UPDATE`, coordinator.instanceKey, request).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return OwnerCommand{}, false, &Error{Kind: ErrorNotFound, Field: "request", Message: "is not found"}
	}
	if err != nil {
		return OwnerCommand{}, false, postgresError("lock rights request", err)
	}
	rows, err := tx.QueryContext(ctx, `
SELECT task_id, owner_key, phase, command
FROM privacy_owner_tasks
WHERE instance_key=$1 AND request_id=$2 AND phase <> 'terminal'
  AND (next_attempt_at IS NULL OR next_attempt_at <= $3)
  AND (lease_until IS NULL OR lease_until <= $3)
ORDER BY task_id
FOR UPDATE SKIP LOCKED`, coordinator.instanceKey, request, now)
	if err != nil {
		return OwnerCommand{}, false, postgresError("query claimable owner tasks", err)
	}
	defer rows.Close()
	type candidate struct {
		id      OwnerTaskID
		owner   OwnerKey
		phase   TaskPhase
		encoded []byte
	}
	var candidates []candidate
	for rows.Next() {
		var value candidate
		if err := rows.Scan(&value.id, &value.owner, &value.phase, &value.encoded); err != nil {
			return OwnerCommand{}, false, postgresError("scan claimable owner task", err)
		}
		candidates = append(candidates, value)
	}
	if err := rows.Err(); err != nil {
		return OwnerCommand{}, false, postgresError("iterate claimable owner tasks", err)
	}
	if err := rows.Close(); err != nil {
		return OwnerCommand{}, false, postgresError("close claimable owner tasks", err)
	}
	nonFinalizersTerminal, err := coordinator.nonFinalizersTerminal(ctx, tx, request)
	if err != nil {
		return OwnerCommand{}, false, err
	}
	var selected *candidate
	for index := range candidates {
		owner := coordinator.catalog.owners[candidates[index].owner]
		if owner.FinalizeAfterOwners && !nonFinalizersTerminal {
			continue
		}
		selected = &candidates[index]
		break
	}
	if selected == nil {
		return OwnerCommand{}, false, nil
	}
	taskID, encoded := selected.id, selected.encoded
	var command OwnerCommand
	if err := json.Unmarshal(encoded, &command); err != nil {
		return OwnerCommand{}, false, postgresError("decode claimed task", err)
	}
	lease := now.Add(coordinator.catalog.limits.TaskLease)
	if _, err := tx.ExecContext(ctx, `
UPDATE privacy_owner_tasks
SET phase='in_flight', attempt=attempt+1, lease_until=$3, next_attempt_at=$3, updated_at=$4
WHERE instance_key=$1 AND task_id=$2`,
		coordinator.instanceKey, taskID, lease, now,
	); err != nil {
		return OwnerCommand{}, false, postgresError("mark task in flight", err)
	}
	if err := tx.Commit(); err != nil {
		return OwnerCommand{}, false, postgresError("commit task claim", err)
	}
	return command, true, nil
}

func (coordinator *PostgresCoordinator) nonFinalizersTerminal(
	ctx context.Context, queryer privacyQueryer, request RightsRequestID,
) (bool, error) {
	rows, err := queryer.QueryContext(ctx, `
SELECT owner_key, phase FROM privacy_owner_tasks
WHERE instance_key=$1 AND request_id=$2`, coordinator.instanceKey, request)
	if err != nil {
		return false, postgresError("query owner finalization gate", err)
	}
	defer rows.Close()
	for rows.Next() {
		var owner OwnerKey
		var phase TaskPhase
		if err := rows.Scan(&owner, &phase); err != nil {
			return false, postgresError("scan owner finalization gate", err)
		}
		if !coordinator.catalog.owners[owner].FinalizeAfterOwners && phase != TaskTerminal {
			return false, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, postgresError("iterate owner finalization gate", err)
	}
	return true, nil
}

func (coordinator *PostgresCoordinator) retry(ctx context.Context, task OwnerTaskID, retryAt *time.Time) error {
	now := coordinator.clock().UTC()
	if retryAt == nil {
		value := now.Add(coordinator.catalog.limits.DefaultRetryDelay)
		retryAt = &value
	}
	_, err := coordinator.db.ExecContext(ctx, `
UPDATE privacy_owner_tasks
SET phase='waiting', lease_until=NULL, next_attempt_at=$3, updated_at=$4
WHERE instance_key=$1 AND task_id=$2 AND phase <> 'terminal'`,
		coordinator.instanceKey, task, retryAt, now)
	if err != nil {
		return postgresError("schedule owner task retry", err)
	}
	return nil
}

func (coordinator *PostgresCoordinator) accept(ctx context.Context, command OwnerCommand, receipt OwnerReceipt) error {
	tx, err := coordinator.db.BeginTx(ctx, nil)
	if err != nil {
		return postgresError("begin owner receipt", err)
	}
	defer func() { _ = tx.Rollback() }()
	var existingFingerprint string
	err = tx.QueryRowContext(ctx, `
SELECT fingerprint FROM privacy_owner_receipts
WHERE instance_key=$1 AND task_id=$2 AND sequence=$3`,
		coordinator.instanceKey, command.TaskID, receipt.Sequence).Scan(&existingFingerprint)
	if err == nil && existingFingerprint != receipt.Fingerprint {
		return &Error{Kind: ErrorProtocolViolation, Field: "owner_receipt", Message: "sequence is reused with a different receipt"}
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return postgresError("load owner receipt sequence", err)
	}
	encoded, _ := json.Marshal(receipt)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO privacy_owner_receipts(instance_key, task_id, sequence, fingerprint, receipt, recorded_at)
VALUES ($1,$2,$3,$4,$5,$6)`,
			coordinator.instanceKey, command.TaskID, receipt.Sequence, receipt.Fingerprint, encoded, receipt.RecordedAt,
		); err != nil {
			return postgresError("insert owner receipt", err)
		}
	}
	phase := TaskWaiting
	next := receipt.RetryAfter
	if receipt.Terminal {
		phase, next = TaskTerminal, nil
	}
	if next == nil && !receipt.Terminal {
		value := coordinator.clock().UTC().Add(coordinator.catalog.limits.DefaultRetryDelay)
		next = &value
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE privacy_owner_tasks
SET phase=$3, lease_until=NULL, next_attempt_at=$4,
    terminal_receipt=CASE WHEN $5 THEN $6 ELSE terminal_receipt END, updated_at=$7
WHERE instance_key=$1 AND task_id=$2`,
		coordinator.instanceKey, command.TaskID, phase, next, receipt.Terminal, encoded, coordinator.clock().UTC(),
	); err != nil {
		return postgresError("apply owner receipt", err)
	}
	if err := coordinator.recomputeRequestTx(ctx, tx, command.RequestID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return postgresError("commit owner receipt", err)
	}
	return nil
}

func (coordinator *PostgresCoordinator) recomputeRequestTx(ctx context.Context, tx *sql.Tx, request RightsRequestID) error {
	var total, terminal int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*), COUNT(*) FILTER (WHERE phase='terminal')
FROM privacy_owner_tasks WHERE instance_key=$1 AND request_id=$2`,
		coordinator.instanceKey, request).Scan(&total, &terminal); err != nil {
		return postgresError("summarize owner tasks", err)
	}
	phase := RequestActive
	if terminal == total {
		phase = RequestComplete
	} else if terminal > 0 {
		phase = RequestPartial
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE privacy_rights_requests SET phase=$3, updated_at=$4
WHERE instance_key=$1 AND request_id=$2`,
		coordinator.instanceKey, request, phase, coordinator.clock().UTC(),
	); err != nil {
		return postgresError("update request phase", err)
	}
	return nil
}
