package privacy

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"
)

type MemoryCoordinatorOptions struct {
	Clock  func() time.Time
	Router OwnerRouter
}

type MemoryCoordinator struct {
	catalog  *Catalog
	clock    func() time.Time
	router   OwnerRouter
	mu       sync.Mutex
	keys     map[IdempotencyKey]memoryRequestKey
	requests map[RightsRequestID]*memoryRequest
}

type memoryRequestKey struct {
	fingerprint string
	id          RightsRequestID
}

type memoryRequest struct {
	view    RightsRequestView
	subject SubjectContext
	tasks   map[OwnerTaskID]*memoryTask
}

type memoryTask struct {
	view    OwnerTaskView
	command OwnerCommand
}

var _ Coordinator = (*MemoryCoordinator)(nil)

func NewMemoryCoordinator(catalog *Catalog, options MemoryCoordinatorOptions) (*MemoryCoordinator, error) {
	if catalog == nil {
		return nil, invalid("catalog", "is required")
	}
	if catalog.owners == nil || len(catalog.owners) == 0 || len(catalog.rights) == 0 {
		return nil, invalid("catalog.coordination", "must declare owners and rights policies")
	}
	if options.Router == nil {
		return nil, invalid("router", "is required")
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	return &MemoryCoordinator{
		catalog: catalog, clock: clock, router: options.Router,
		keys: map[IdempotencyKey]memoryRequestKey{}, requests: map[RightsRequestID]*memoryRequest{},
	}, nil
}

func (coordinator *MemoryCoordinator) Open(ctx context.Context, command OpenRightsRequest) (RightsRequestView, error) {
	if err := ctx.Err(); err != nil {
		return RightsRequestView{}, err
	}
	now := coordinator.clock().UTC()
	command, policy, owners, err := coordinator.prepareOpen(command, now)
	if err != nil {
		return RightsRequestView{}, err
	}
	commandFingerprint := fingerprint(command)
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if existing, exists := coordinator.keys[command.IdempotencyKey]; exists {
		if existing.fingerprint != commandFingerprint {
			return RightsRequestView{}, conflict("idempotency_key", "is reused with a different request")
		}
		view := coordinator.copyView(coordinator.requests[existing.id])
		view.Replay = true
		return view, nil
	}
	id := RightsRequestID(receiptID("request", commandFingerprint))
	request := &memoryRequest{
		subject: command.Subject, tasks: map[OwnerTaskID]*memoryTask{},
		view: RightsRequestView{
			ID: id, Operation: command.Operation, Phase: RequestOpen,
			Deadline:    policy.RespondWithin.Add(command.RequestedAt).UTC(),
			RequestedAt: command.RequestedAt, UpdatedAt: now, Fingerprint: commandFingerprint,
		},
	}
	for _, owner := range owners {
		datasets := datasetsForOperation(owner, command.Operation)
		taskID := OwnerTaskID(receiptID("task", fingerprint(struct {
			Request   RightsRequestID
			Owner     OwnerRef
			Operation RightsOperation
		}{id, owner.Ref, command.Operation})))
		ownerCommand := OwnerCommand{
			ProtocolVersion: OwnerProtocolVersion, RequestID: id, TaskID: taskID,
			Owner: owner.Ref, Operation: command.Operation, Subject: command.Subject,
			Datasets: datasets, RequestedAt: command.RequestedAt, Deadline: request.view.Deadline,
		}
		ownerCommand.Fingerprint = commandFingerprintForOwner(ownerCommand)
		request.tasks[taskID] = &memoryTask{
			command: ownerCommand,
			view:    OwnerTaskView{ID: taskID, Owner: owner.Ref, Phase: TaskPending},
		}
	}
	coordinator.recompute(request, now)
	coordinator.keys[command.IdempotencyKey] = memoryRequestKey{fingerprint: commandFingerprint, id: id}
	coordinator.requests[id] = request
	return coordinator.copyView(request), nil
}

func (coordinator *MemoryCoordinator) Get(ctx context.Context, id RightsRequestID) (RightsRequestView, error) {
	if err := ctx.Err(); err != nil {
		return RightsRequestView{}, err
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	request, exists := coordinator.requests[id]
	if !exists {
		return RightsRequestView{}, &Error{Kind: ErrorNotFound, Field: "request", Message: "is not found"}
	}
	coordinator.recompute(request, coordinator.clock().UTC())
	return coordinator.copyView(request), nil
}

func (coordinator *MemoryCoordinator) Drive(ctx context.Context, command DriveRightsRequest) (DriveResult, error) {
	if err := ctx.Err(); err != nil {
		return DriveResult{}, err
	}
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
	deadline := coordinator.clock().Add(budget.MaxDuration)
	attempts := 0
	for attempts < budget.MaxOwnerAttempts && coordinator.clock().Before(deadline) {
		task, err := coordinator.claim(command.Request)
		if err != nil {
			return DriveResult{}, err
		}
		if task == nil {
			break
		}
		attempts++
		host, routeErr := coordinator.router.Owner(ctx, task.command.Owner.Key)
		if routeErr != nil {
			coordinator.retryTask(command.Request, task.command.TaskID, routeErr, nil)
			continue
		}
		receipt, ownerErr := host.Handle(ctx, task.command)
		if ownerErr != nil {
			coordinator.retryTask(command.Request, task.command.TaskID, ownerErr, nil)
			continue
		}
		if err := validateOwnerReceipt(task.command, receipt); err != nil {
			coordinator.retryTask(command.Request, task.command.TaskID, err, nil)
			return DriveResult{}, err
		}
		coordinator.acceptReceipt(command.Request, task.command.TaskID, receipt)
	}
	view, err := coordinator.Get(ctx, command.Request)
	if err != nil {
		return DriveResult{}, err
	}
	var next *time.Time
	for _, task := range view.Tasks {
		if task.Phase == TaskWaiting || task.Phase == TaskInFlight {
			if task.NextAttemptAt != nil && (next == nil || task.NextAttemptAt.Before(*next)) {
				value := *task.NextAttemptAt
				next = &value
			}
		}
	}
	return DriveResult{View: view, NextAttemptAt: next}, nil
}

func (coordinator *MemoryCoordinator) claim(requestID RightsRequestID) (*memoryTask, error) {
	now := coordinator.clock().UTC()
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	request, exists := coordinator.requests[requestID]
	if !exists {
		return nil, &Error{Kind: ErrorNotFound, Field: "request", Message: "is not found"}
	}
	ids := make([]OwnerTaskID, 0, len(request.tasks))
	for id := range request.tasks {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		task := request.tasks[id]
		if task.view.Phase == TaskTerminal {
			continue
		}
		if coordinator.isFinalizer(task.command.Owner.Key) && !coordinator.nonFinalizersTerminal(request) {
			continue
		}
		if task.view.NextAttemptAt != nil && now.Before(*task.view.NextAttemptAt) {
			continue
		}
		task.view.Phase = TaskInFlight
		task.view.Attempt++
		lease := now.Add(coordinator.catalog.limits.TaskLease)
		task.view.NextAttemptAt = &lease
		request.view.UpdatedAt = now
		value := *task
		value.command.Deadline = request.view.Deadline
		return &value, nil
	}
	return nil, nil
}

func (coordinator *MemoryCoordinator) isFinalizer(owner OwnerKey) bool {
	definition, exists := coordinator.catalog.owners[owner]
	return exists && definition.FinalizeAfterOwners
}

func (coordinator *MemoryCoordinator) nonFinalizersTerminal(request *memoryRequest) bool {
	for _, task := range request.tasks {
		if coordinator.isFinalizer(task.command.Owner.Key) {
			continue
		}
		if task.view.Phase != TaskTerminal {
			return false
		}
	}
	return true
}

func (coordinator *MemoryCoordinator) retryTask(requestID RightsRequestID, taskID OwnerTaskID, cause error, retryAt *time.Time) {
	now := coordinator.clock().UTC()
	if retryAt == nil {
		value := now.Add(coordinator.catalog.limits.DefaultRetryDelay)
		retryAt = &value
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	request := coordinator.requests[requestID]
	if request == nil {
		return
	}
	task := request.tasks[taskID]
	if task == nil || task.view.Phase == TaskTerminal {
		return
	}
	task.view.Phase, task.view.NextAttemptAt = TaskWaiting, retryAt
	request.view.UpdatedAt = now
	coordinator.recompute(request, now)
	_ = cause // the durable Postgres adapter records bounded attempt diagnostics.
}

func (coordinator *MemoryCoordinator) acceptReceipt(requestID RightsRequestID, taskID OwnerTaskID, receipt OwnerReceipt) {
	now := coordinator.clock().UTC()
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	request := coordinator.requests[requestID]
	task := request.tasks[taskID]
	if task.view.Receipt != nil && task.view.Receipt.Fingerprint != receipt.Fingerprint {
		return
	}
	task.view.Receipt = &receipt
	if receipt.Terminal {
		task.view.Phase, task.view.NextAttemptAt = TaskTerminal, nil
	} else {
		task.view.Phase, task.view.NextAttemptAt = TaskWaiting, receipt.RetryAfter
		if task.view.NextAttemptAt == nil {
			value := now.Add(coordinator.catalog.limits.DefaultRetryDelay)
			task.view.NextAttemptAt = &value
		}
	}
	request.view.UpdatedAt = now
	coordinator.recompute(request, now)
}

func (coordinator *MemoryCoordinator) prepareOpen(
	command OpenRightsRequest,
	now time.Time,
) (OpenRightsRequest, RightsPolicy, []OwnerDefinition, error) {
	const allowedClockSkew = 5 * time.Minute
	now = now.UTC()
	key := strings.TrimSpace(string(command.IdempotencyKey))
	if key == "" || len(key) > coordinator.catalog.limits.MaxIdempotencyBytes {
		return command, RightsPolicy{}, nil, invalid("idempotency_key", "is invalid")
	}
	policy, exists := coordinator.catalog.rights[command.Operation]
	if !exists {
		return command, RightsPolicy{}, nil, &Error{Kind: ErrorNotFound, Field: "operation", Message: "is not configured"}
	}
	command.Verification.VerifiedAt = command.Verification.VerifiedAt.UTC()
	if command.Verification.VerifiedAt.IsZero() || command.Verification.Method == "" || command.Verification.VerificationRef == "" ||
		now.Sub(command.Verification.VerifiedAt) > policy.VerificationMaxAge ||
		command.Verification.VerifiedAt.After(now.Add(allowedClockSkew)) {
		return command, RightsPolicy{}, nil, invalid("verification", "is missing, expired, or incomplete")
	}
	if command.RequestedAt.IsZero() {
		command.RequestedAt = now
	}
	command.RequestedAt = command.RequestedAt.UTC()
	if command.RequestedAt.After(now.Add(allowedClockSkew)) ||
		command.Verification.VerifiedAt.After(command.RequestedAt.Add(allowedClockSkew)) {
		return command, RightsPolicy{}, nil, invalid("requested_at", "is in the future or precedes verification")
	}
	if command.Subject.Current == nil && len(command.Subject.Aliases) == 0 {
		return command, RightsPolicy{}, nil, invalid("subject", "must contain at least one reference")
	}
	if len(command.Subject.Aliases) > coordinator.catalog.limits.MaxAliases {
		return command, RightsPolicy{}, nil, invalid("subject.aliases", "exceeds the configured limit")
	}
	var owners []OwnerDefinition
	for _, owner := range coordinator.catalog.owners {
		if len(datasetsForOperation(owner, command.Operation)) > 0 {
			owners = append(owners, owner)
		}
	}
	if len(owners) == 0 {
		return command, RightsPolicy{}, nil, invalid("operation", "has no capable owners")
	}
	slices.SortFunc(owners, func(a, b OwnerDefinition) int { return strings.Compare(string(a.Ref.Key), string(b.Ref.Key)) })
	return command, policy, owners, nil
}

func (coordinator *MemoryCoordinator) recompute(request *memoryRequest, now time.Time) {
	summary := RightsSummary{}
	terminal := 0
	for _, task := range request.tasks {
		if task.view.Phase != TaskTerminal || task.view.Receipt == nil {
			summary.Pending++
			continue
		}
		terminal++
		for _, result := range task.view.Receipt.Results {
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
	switch {
	case terminal == len(request.tasks):
		request.view.Phase = RequestComplete
	case terminal > 0:
		request.view.Phase = RequestPartial
	case request.view.Phase == RequestOpen:
		request.view.Phase = RequestOpen
	default:
		request.view.Phase = RequestActive
	}
	request.view.Overdue = now.After(request.view.Deadline) && request.view.Phase != RequestComplete
	request.view.Summary = summary
}

func (coordinator *MemoryCoordinator) copyView(request *memoryRequest) RightsRequestView {
	view := request.view
	view.Tasks = make([]OwnerTaskView, 0, len(request.tasks))
	for _, task := range request.tasks {
		item := task.view
		if item.Receipt != nil {
			copy := *item.Receipt
			copy.Results = append([]DatasetOutcome(nil), item.Receipt.Results...)
			item.Receipt = &copy
		}
		view.Tasks = append(view.Tasks, item)
	}
	slices.SortFunc(view.Tasks, func(a, b OwnerTaskView) int { return strings.Compare(string(a.ID), string(b.ID)) })
	return view
}

func datasetsForOperation(owner OwnerDefinition, operation RightsOperation) []DatasetKey {
	var result []DatasetKey
	for _, dataset := range owner.Datasets {
		if slices.Contains(dataset.Operations, operation) {
			result = append(result, dataset.Key)
		}
	}
	slices.Sort(result)
	return result
}

func commandFingerprintForOwner(command OwnerCommand) string {
	copy := command
	copy.Fingerprint = ""
	return fingerprint(copy)
}

func validateOwnerReceipt(command OwnerCommand, receipt OwnerReceipt) error {
	if receipt.ProtocolVersion != OwnerProtocolVersion || receipt.RequestID != command.RequestID ||
		receipt.TaskID != command.TaskID || receipt.Owner != command.Owner ||
		receipt.CommandFingerprint != command.Fingerprint || receipt.Sequence == 0 ||
		receipt.Fingerprint == "" {
		return &Error{Kind: ErrorProtocolViolation, Field: "owner_receipt", Message: "does not match the command"}
	}
	if receipt.Terminal {
		if len(receipt.Results) != len(command.Datasets) {
			return &Error{Kind: ErrorProtocolViolation, Field: "owner_receipt.results", Message: "must account for every dataset"}
		}
		seen := map[DatasetKey]struct{}{}
		for _, result := range receipt.Results {
			if !slices.Contains(command.Datasets, result.Dataset) || !validDisposition(result.Disposition) {
				return &Error{Kind: ErrorProtocolViolation, Field: "owner_receipt.results", Message: "contains an invalid result"}
			}
			if _, exists := seen[result.Dataset]; exists {
				return &Error{Kind: ErrorProtocolViolation, Field: "owner_receipt.results", Message: "contains a duplicate dataset"}
			}
			if (result.Disposition == DispositionRetained || result.Disposition == DispositionRefused) && result.Reason == "" {
				return &Error{Kind: ErrorProtocolViolation, Field: "owner_receipt.results", Message: "retained/refused requires a reason"}
			}
			seen[result.Dataset] = struct{}{}
		}
	}
	copy := receipt
	copy.Fingerprint, copy.Replay = "", false
	if fingerprint(copy) != receipt.Fingerprint {
		return &Error{Kind: ErrorProtocolViolation, Field: "owner_receipt.fingerprint", Message: "does not match the receipt"}
	}
	return nil
}

func ownerReceiptFingerprint(receipt OwnerReceipt) string {
	receipt.Fingerprint, receipt.Replay = "", false
	return fingerprint(receipt)
}
