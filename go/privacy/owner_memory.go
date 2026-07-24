package privacy

import (
	"context"
	"slices"
	"sync"
	"time"
)

type MemoryOwnerHostOptions struct {
	Clock    func() time.Time
	Executor OwnerExecutor
}

type MemoryOwnerHost struct {
	owner    OwnerDefinition
	clock    func() time.Time
	executor OwnerExecutor
	mu       sync.Mutex
	commands map[OwnerTaskID]string
	receipts map[OwnerTaskID]OwnerReceipt
	attempts map[OwnerTaskID]uint32
}

var _ OwnerHost = (*MemoryOwnerHost)(nil)

func NewMemoryOwnerHost(owner OwnerDefinition, options MemoryOwnerHostOptions) (*MemoryOwnerHost, error) {
	if options.Executor == nil {
		return nil, invalid("executor", "is required")
	}
	if owner.Ref.Key == "" || owner.Ref.Revision == 0 || owner.Ref.Digest == "" {
		return nil, invalid("owner", "must be a compiled owner definition")
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	return &MemoryOwnerHost{
		owner: owner, clock: clock, executor: options.Executor,
		commands: map[OwnerTaskID]string{}, receipts: map[OwnerTaskID]OwnerReceipt{},
		attempts: map[OwnerTaskID]uint32{},
	}, nil
}

func (host *MemoryOwnerHost) Handle(ctx context.Context, command OwnerCommand) (OwnerReceipt, error) {
	if err := ctx.Err(); err != nil {
		return OwnerReceipt{}, err
	}
	if err := host.validateCommand(command); err != nil {
		return OwnerReceipt{}, err
	}
	host.mu.Lock()
	if existing, exists := host.commands[command.TaskID]; exists {
		if existing != command.Fingerprint {
			host.mu.Unlock()
			return OwnerReceipt{}, conflict("task_id", "is reused with a different command")
		}
		if receipt, exists := host.receipts[command.TaskID]; exists && receipt.Terminal {
			receipt.Replay = true
			host.mu.Unlock()
			return receipt, nil
		}
	} else {
		host.commands[command.TaskID] = command.Fingerprint
	}
	host.attempts[command.TaskID]++
	attempt := host.attempts[command.TaskID]
	host.mu.Unlock()

	outcome, err := host.executor.Execute(ctx, OwnerInstruction{Command: command, Attempt: attempt})
	if err != nil {
		return OwnerReceipt{}, err
	}
	if err := host.validateOutcome(command, outcome); err != nil {
		return OwnerReceipt{}, err
	}

	host.mu.Lock()
	defer host.mu.Unlock()
	if existing, exists := host.receipts[command.TaskID]; exists && existing.Terminal {
		existing.Replay = true
		return existing, nil
	}
	sequence := uint64(1)
	if existing, exists := host.receipts[command.TaskID]; exists {
		sequence = existing.Sequence + 1
	}
	receipt := OwnerReceipt{
		ProtocolVersion: OwnerProtocolVersion, ID: string(receiptID("owner", fingerprint(struct {
			Task     OwnerTaskID
			Sequence uint64
		}{command.TaskID, sequence}))),
		RequestID: command.RequestID, TaskID: command.TaskID, Owner: command.Owner,
		CommandFingerprint: command.Fingerprint, Sequence: sequence, Terminal: outcome.Terminal,
		Results: append([]DatasetOutcome(nil), outcome.Results...), RetryAfter: outcome.RetryAfter,
		RecordedAt: host.clock().UTC(),
	}
	receipt.Fingerprint = ownerReceiptFingerprint(receipt)
	host.receipts[command.TaskID] = receipt
	return receipt, nil
}

func (host *MemoryOwnerHost) validateCommand(command OwnerCommand) error {
	if command.ProtocolVersion != OwnerProtocolVersion || command.TaskID == "" || command.RequestID == "" ||
		command.Owner != host.owner.Ref || command.Fingerprint == "" ||
		commandFingerprintForOwner(command) != command.Fingerprint || !validRightsOperation(command.Operation) {
		return &Error{Kind: ErrorProtocolViolation, Field: "owner_command", Message: "is invalid"}
	}
	expected := datasetsForOperation(host.owner, command.Operation)
	if !slices.Equal(expected, command.Datasets) {
		return &Error{Kind: ErrorProtocolViolation, Field: "owner_command.datasets", Message: "does not match owner capabilities"}
	}
	return nil
}

func (host *MemoryOwnerHost) validateOutcome(command OwnerCommand, outcome OwnerOutcome) error {
	if !outcome.Terminal {
		if outcome.RetryAfter == nil {
			return &Error{Kind: ErrorProtocolViolation, Field: "owner_outcome.retry_after", Message: "is required for non-terminal outcomes"}
		}
		return nil
	}
	if len(outcome.Results) != len(command.Datasets) {
		return &Error{Kind: ErrorProtocolViolation, Field: "owner_outcome.results", Message: "must account for every dataset"}
	}
	seen := map[DatasetKey]struct{}{}
	for _, result := range outcome.Results {
		if !slices.Contains(command.Datasets, result.Dataset) || !validDisposition(result.Disposition) {
			return &Error{Kind: ErrorProtocolViolation, Field: "owner_outcome.results", Message: "contains an invalid result"}
		}
		if _, exists := seen[result.Dataset]; exists {
			return &Error{Kind: ErrorProtocolViolation, Field: "owner_outcome.results", Message: "contains a duplicate dataset"}
		}
		seen[result.Dataset] = struct{}{}
		if (result.Disposition == DispositionRetained || result.Disposition == DispositionRefused) && result.Reason == "" {
			return &Error{Kind: ErrorProtocolViolation, Field: "owner_outcome.results", Message: "retained/refused requires a reason"}
		}
		for _, artifact := range result.Artifacts {
			if artifact.Provider == "" || artifact.Key == "" || artifact.Digest == "" || artifact.ExpiresAt.IsZero() {
				return &Error{Kind: ErrorProtocolViolation, Field: "owner_outcome.artifacts", Message: "contains an invalid artifact reference"}
			}
		}
	}
	return nil
}
