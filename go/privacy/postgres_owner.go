package privacy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type PostgresOwnerHost struct {
	db          *sql.DB
	instanceKey string
	owner       OwnerDefinition
	executor    OwnerExecutor
	clock       func() time.Time
}

var _ OwnerHost = (*PostgresOwnerHost)(nil)

func NewPostgresOwnerHost(
	ctx context.Context,
	catalog *Catalog,
	options PostgresOptions,
	executor OwnerExecutor,
) (*PostgresOwnerHost, error) {
	if executor == nil {
		return nil, invalid("executor", "is required")
	}
	owner, exists := catalog.Owner()
	if !exists {
		return nil, invalid("catalog.owner", "must declare the local owner")
	}
	runtime, err := NewPostgresRuntime(ctx, catalog, options)
	if err != nil {
		return nil, err
	}
	return &PostgresOwnerHost{
		db: runtime.db, instanceKey: runtime.instanceKey, owner: owner,
		executor: executor, clock: runtime.clock,
	}, nil
}

func (host *PostgresOwnerHost) Handle(ctx context.Context, command OwnerCommand) (OwnerReceipt, error) {
	validator := &MemoryOwnerHost{owner: host.owner}
	if err := validator.validateCommand(command); err != nil {
		return OwnerReceipt{}, err
	}
	attempt, existing, err := host.acceptCommand(ctx, command)
	if err != nil {
		return OwnerReceipt{}, err
	}
	if existing != nil {
		existing.Replay = true
		return *existing, nil
	}
	outcome, err := host.executor.Execute(ctx, OwnerInstruction{Command: command, Attempt: attempt})
	if err != nil {
		return OwnerReceipt{}, err
	}
	if err := validator.validateOutcome(command, outcome); err != nil {
		return OwnerReceipt{}, err
	}
	return host.recordOutcome(ctx, command, outcome)
}

func (host *PostgresOwnerHost) acceptCommand(
	ctx context.Context,
	command OwnerCommand,
) (uint32, *OwnerReceipt, error) {
	tx, err := host.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, postgresError("begin owner command", err)
	}
	defer func() { _ = tx.Rollback() }()
	encoded, _ := json.Marshal(command)
	now := host.clock().UTC()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO privacy_host_commands(
    instance_key, owner_key, task_id, command_fingerprint, command, updated_at
) VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (instance_key, owner_key, task_id) DO NOTHING`,
		host.instanceKey, host.owner.Ref.Key, command.TaskID, command.Fingerprint, encoded, now,
	); err != nil {
		return 0, nil, postgresError("accept owner command", err)
	}
	var storedFingerprint string
	var attempt uint32
	var terminal bool
	var receiptJSON []byte
	if err := tx.QueryRowContext(ctx, `
SELECT command_fingerprint, attempt, terminal, latest_receipt
FROM privacy_host_commands
WHERE instance_key=$1 AND owner_key=$2 AND task_id=$3
FOR UPDATE`,
		host.instanceKey, host.owner.Ref.Key, command.TaskID,
	).Scan(&storedFingerprint, &attempt, &terminal, &receiptJSON); err != nil {
		return 0, nil, postgresError("load owner command", err)
	}
	if storedFingerprint != command.Fingerprint {
		return 0, nil, conflict("task_id", "is reused with a different command")
	}
	if terminal && len(receiptJSON) > 0 {
		var receipt OwnerReceipt
		if err := json.Unmarshal(receiptJSON, &receipt); err != nil {
			return 0, nil, postgresError("decode terminal owner receipt", err)
		}
		if err := tx.Commit(); err != nil {
			return 0, nil, postgresError("commit owner replay", err)
		}
		return attempt, &receipt, nil
	}
	attempt++
	if _, err := tx.ExecContext(ctx, `
UPDATE privacy_host_commands SET attempt=$4, updated_at=$5
WHERE instance_key=$1 AND owner_key=$2 AND task_id=$3`,
		host.instanceKey, host.owner.Ref.Key, command.TaskID, attempt, now,
	); err != nil {
		return 0, nil, postgresError("increment owner attempt", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, nil, postgresError("commit owner command", err)
	}
	return attempt, nil, nil
}

func (host *PostgresOwnerHost) recordOutcome(
	ctx context.Context,
	command OwnerCommand,
	outcome OwnerOutcome,
) (OwnerReceipt, error) {
	tx, err := host.db.BeginTx(ctx, nil)
	if err != nil {
		return OwnerReceipt{}, postgresError("begin owner outcome", err)
	}
	defer func() { _ = tx.Rollback() }()
	var terminal bool
	var previousJSON []byte
	if err := tx.QueryRowContext(ctx, `
SELECT terminal, latest_receipt
FROM privacy_host_commands
WHERE instance_key=$1 AND owner_key=$2 AND task_id=$3
FOR UPDATE`,
		host.instanceKey, host.owner.Ref.Key, command.TaskID,
	).Scan(&terminal, &previousJSON); err != nil {
		return OwnerReceipt{}, postgresError("lock owner command", err)
	}
	if terminal && len(previousJSON) > 0 {
		var previous OwnerReceipt
		if err := json.Unmarshal(previousJSON, &previous); err != nil {
			return OwnerReceipt{}, postgresError("decode owner replay", err)
		}
		previous.Replay = true
		if err := tx.Commit(); err != nil {
			return OwnerReceipt{}, postgresError("commit owner outcome replay", err)
		}
		return previous, nil
	}
	var sequence uint64
	err = tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(sequence), 0)
FROM privacy_host_receipts
WHERE instance_key=$1 AND owner_key=$2 AND task_id=$3`,
		host.instanceKey, host.owner.Ref.Key, command.TaskID).Scan(&sequence)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return OwnerReceipt{}, postgresError("load owner receipt sequence", err)
	}
	sequence++
	receipt := OwnerReceipt{
		ProtocolVersion: OwnerProtocolVersion,
		ID: string(receiptID("owner", fingerprint(struct {
			Task     OwnerTaskID
			Sequence uint64
		}{command.TaskID, sequence}))),
		RequestID: command.RequestID, TaskID: command.TaskID, Owner: command.Owner,
		CommandFingerprint: command.Fingerprint, Sequence: sequence, Terminal: outcome.Terminal,
		Results: append([]DatasetOutcome(nil), outcome.Results...), RetryAfter: outcome.RetryAfter,
		RecordedAt: host.clock().UTC(),
	}
	receipt.Fingerprint = ownerReceiptFingerprint(receipt)
	encoded, _ := json.Marshal(receipt)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO privacy_host_receipts(
    instance_key, owner_key, task_id, sequence, fingerprint, receipt, recorded_at
) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		host.instanceKey, host.owner.Ref.Key, command.TaskID, sequence,
		receipt.Fingerprint, encoded, receipt.RecordedAt,
	); err != nil {
		return OwnerReceipt{}, postgresError("insert host receipt", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE privacy_host_commands
SET terminal=$4, latest_receipt=$5, updated_at=$6
WHERE instance_key=$1 AND owner_key=$2 AND task_id=$3`,
		host.instanceKey, host.owner.Ref.Key, command.TaskID, receipt.Terminal, encoded, host.clock().UTC(),
	); err != nil {
		return OwnerReceipt{}, postgresError("update host receipt", err)
	}
	if err := tx.Commit(); err != nil {
		return OwnerReceipt{}, postgresError("commit owner outcome", err)
	}
	return receipt, nil
}
