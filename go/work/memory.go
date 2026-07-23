package work

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"
)

type MemoryOptions struct {
	Clock func() time.Time
}

type memoryJob struct {
	job         Job
	fingerprint [32]byte
	leaseToken  string
	pausedFrom  Status
}

type idempotencyRecord struct {
	jobID       JobID
	fingerprint [32]byte
}

// Memory is the deterministic reference Adapter.
type Memory struct {
	mu             sync.RWMutex
	catalog        *Catalog
	clock          func() time.Time
	jobs           map[JobID]*memoryJob
	attempts       map[JobID][]Attempt
	idempotency    map[string]idempotencyRecord
	scheduleCursor map[string]time.Time
}

var _ Backend = (*Memory)(nil)

func NewMemory(catalog *Catalog, options MemoryOptions) (*Memory, error) {
	if catalog == nil {
		return nil, invalid("catalog", "is required")
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	now := clock().UTC()
	memory := &Memory{
		catalog: catalog, clock: clock, jobs: make(map[JobID]*memoryJob),
		attempts: make(map[JobID][]Attempt), idempotency: make(map[string]idempotencyRecord),
		scheduleCursor: make(map[string]time.Time),
	}
	for key := range catalog.schedules {
		next, _ := catalog.nextSchedule(key, now)
		memory.scheduleCursor[key] = next
	}
	return memory, nil
}

func (memory *Memory) Enqueue(ctx context.Context, request Request) (EnqueueResult, error) {
	if err := ctx.Err(); err != nil {
		return EnqueueResult{}, err
	}
	now := memory.clock().UTC()
	prepared, err := memory.catalog.Prepare(now, request)
	if err != nil {
		return EnqueueResult{}, err
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	return memory.enqueueLocked(now, prepared, "")
}

func (memory *Memory) enqueueLocked(now time.Time, prepared PreparedRequest, replayOf JobID) (EnqueueResult, error) {
	if prepared.IdempotencyKey != "" {
		if existing, ok := memory.idempotency[prepared.IdempotencyKey]; ok {
			if existing.fingerprint != prepared.Fingerprint {
				return EnqueueResult{}, conflict("idempotency_key", "%q is reused with a different request", prepared.IdempotencyKey)
			}
			return EnqueueResult{Job: cloneJob(memory.jobs[existing.jobID].job), Replay: true}, nil
		}
	}
	id, err := NewJobID()
	if err != nil {
		return EnqueueResult{}, err
	}
	job := Job{
		ID: id, Kind: prepared.Kind, Queue: prepared.Queue, Status: StatusQueued,
		Payload: cloneJSON(prepared.Payload), Metadata: cloneJSON(prepared.Metadata),
		MaxAttempts: prepared.MaxAttempts, Priority: prepared.Priority, RunAt: prepared.RunAt,
		CreatedAt: now, UpdatedAt: now, ReplayOf: replayOf,
		IdempotencyKey: prepared.IdempotencyKey,
	}
	memory.jobs[id] = &memoryJob{job: job, fingerprint: prepared.Fingerprint}
	if prepared.IdempotencyKey != "" {
		memory.idempotency[prepared.IdempotencyKey] = idempotencyRecord{
			jobID: id, fingerprint: prepared.Fingerprint,
		}
	}
	return EnqueueResult{Job: cloneJob(job)}, nil
}

func (memory *Memory) Get(ctx context.Context, id JobID) (Job, error) {
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	value, ok := memory.jobs[id]
	if !ok {
		return Job{}, notFound("job_id", "%q does not exist", id)
	}
	return cloneJob(value.job), nil
}

func (memory *Memory) List(ctx context.Context, query ListQuery) ([]Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit, err := normalizeList(query)
	if err != nil {
		return nil, err
	}
	statuses, err := statusSet(query.Statuses)
	if err != nil {
		return nil, err
	}
	kinds := make(map[Kind]struct{}, len(query.Kinds))
	for _, kind := range query.Kinds {
		if _, ok := memory.catalog.kinds[kind]; !ok {
			return nil, invalid("kinds", "%q is not registered", kind)
		}
		kinds[kind] = struct{}{}
	}
	queues := make(map[Queue]struct{}, len(query.Queues))
	for _, queue := range query.Queues {
		if _, ok := memory.catalog.queues[queue]; !ok {
			return nil, invalid("queues", "%q is not registered", queue)
		}
		queues[queue] = struct{}{}
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	result := make([]Job, 0)
	for _, value := range memory.jobs {
		if len(statuses) > 0 {
			if _, ok := statuses[value.job.Status]; !ok {
				continue
			}
		}
		if len(kinds) > 0 {
			if _, ok := kinds[value.job.Kind]; !ok {
				continue
			}
		}
		if len(queues) > 0 {
			if _, ok := queues[value.job.Queue]; !ok {
				continue
			}
		}
		result = append(result, cloneJob(value.job))
	}
	slices.SortFunc(result, func(a, b Job) int {
		if order := b.CreatedAt.Compare(a.CreatedAt); order != 0 {
			return order
		}
		return strings.Compare(string(b.ID), string(a.ID))
	})
	if query.Offset >= len(result) {
		return []Job{}, nil
	}
	result = result[query.Offset:]
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func normalizeList(query ListQuery) (int, error) {
	limit := query.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 1000 {
		return 0, invalid("limit", "must be between 1 and 1000")
	}
	if query.Offset < 0 || query.Offset > 1_000_000 {
		return 0, invalid("offset", "must be between 0 and 1000000")
	}
	return limit, nil
}

func statusSet(values []Status) (map[Status]struct{}, error) {
	result := make(map[Status]struct{}, len(values))
	for _, value := range values {
		if !validStatus(value) {
			return nil, invalid("statuses", "%q is unknown", value)
		}
		result[value] = struct{}{}
	}
	return result, nil
}

func validStatus(value Status) bool {
	switch value {
	case StatusQueued, StatusRunning, StatusRetrying, StatusPaused,
		StatusSucceeded, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

func (memory *Memory) Attempts(ctx context.Context, id JobID) ([]Attempt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	if _, ok := memory.jobs[id]; !ok {
		return nil, notFound("job_id", "%q does not exist", id)
	}
	return append([]Attempt(nil), memory.attempts[id]...), nil
}

func (memory *Memory) Stats(ctx context.Context) (Stats, error) {
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}
	now := memory.clock().UTC()
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	result := Stats{ByStatus: make(map[Status]int64)}
	for _, value := range memory.jobs {
		result.ByStatus[value.job.Status]++
		if value.job.Status == StatusRunning {
			result.Running++
		}
		if (value.job.Status == StatusQueued || value.job.Status == StatusRetrying) && !value.job.RunAt.After(now) {
			result.Due++
			if result.OldestDueAt == nil || value.job.RunAt.Before(*result.OldestDueAt) {
				at := value.job.RunAt
				result.OldestDueAt = &at
			}
		}
	}
	return result, nil
}

func (memory *Memory) Pause(ctx context.Context, id JobID) (Job, error) {
	return memory.changeState(ctx, id, "pause")
}

func (memory *Memory) Resume(ctx context.Context, id JobID) (Job, error) {
	return memory.changeState(ctx, id, "resume")
}

func (memory *Memory) Cancel(ctx context.Context, id JobID) (Job, error) {
	return memory.changeState(ctx, id, "cancel")
}

func (memory *Memory) changeState(ctx context.Context, id JobID, operation string) (Job, error) {
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	now := memory.clock().UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	value, ok := memory.jobs[id]
	if !ok {
		return Job{}, notFound("job_id", "%q does not exist", id)
	}
	switch operation {
	case "pause":
		switch value.job.Status {
		case StatusQueued, StatusRetrying, StatusRunning:
			value.pausedFrom = value.job.Status
			if value.job.Status == StatusRunning {
				memory.finishAttemptLocked(value, AttemptPaused, "", now)
			}
			value.job.Status = StatusPaused
			memory.clearLeaseLocked(value)
		default:
			return Job{}, conflict("status", "cannot pause %s job", value.job.Status)
		}
	case "resume":
		if value.job.Status != StatusPaused {
			return Job{}, conflict("status", "cannot resume %s job", value.job.Status)
		}
		if value.pausedFrom == StatusRetrying || value.pausedFrom == StatusRunning || value.job.Attempt > 0 {
			value.job.Status = StatusRetrying
		} else {
			value.job.Status = StatusQueued
		}
		value.pausedFrom = ""
	case "cancel":
		switch value.job.Status {
		case StatusQueued, StatusRetrying, StatusPaused, StatusRunning:
			if value.job.Status == StatusRunning {
				memory.finishAttemptLocked(value, AttemptCancelled, "", now)
			}
			value.job.Status = StatusCancelled
			value.job.CancelledAt = timePtr(now)
			value.job.FinishedAt = timePtr(now)
			memory.clearLeaseLocked(value)
		default:
			return Job{}, conflict("status", "cannot cancel %s job", value.job.Status)
		}
	}
	value.job.UpdatedAt = now
	return cloneJob(value.job), nil
}

func (memory *Memory) Replay(ctx context.Context, id JobID, command ReplayRequest) (EnqueueResult, error) {
	if err := ctx.Err(); err != nil {
		return EnqueueResult{}, err
	}
	now := memory.clock().UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	source, ok := memory.jobs[id]
	if !ok {
		return EnqueueResult{}, notFound("job_id", "%q does not exist", id)
	}
	if source.job.Status != StatusSucceeded && source.job.Status != StatusFailed && source.job.Status != StatusCancelled {
		return EnqueueResult{}, conflict("status", "only terminal jobs can be replayed")
	}
	priority := source.job.Priority
	if command.Priority != nil {
		priority = *command.Priority
	}
	attempts := command.MaxAttempts
	if attempts == 0 {
		attempts = source.job.MaxAttempts
	}
	request := Request{
		Kind: source.job.Kind, Payload: source.job.Payload, Metadata: source.job.Metadata,
		RunAt: command.RunAt, Priority: priority, MaxAttempts: attempts,
		IdempotencyKey: command.IdempotencyKey,
	}
	prepared, err := memory.catalog.Prepare(now, request)
	if err != nil {
		return EnqueueResult{}, err
	}
	prepared, err = memory.catalog.PrepareReplay(prepared, id)
	if err != nil {
		return EnqueueResult{}, err
	}
	return memory.enqueueLocked(now, prepared, id)
}

func (memory *Memory) Prune(ctx context.Context, before time.Time) (PruneResult, error) {
	if err := ctx.Err(); err != nil {
		return PruneResult{}, err
	}
	if before.IsZero() {
		return PruneResult{}, invalid("before", "is required")
	}
	before = before.UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	var result PruneResult
	for id, value := range memory.jobs {
		if !terminal(value.job.Status) || value.job.FinishedAt == nil || !value.job.FinishedAt.Before(before) {
			continue
		}
		result.AttemptsRemoved += int64(len(memory.attempts[id]))
		delete(memory.attempts, id)
		delete(memory.jobs, id)
		if value.job.IdempotencyKey != "" {
			delete(memory.idempotency, value.job.IdempotencyKey)
		}
		result.JobsRemoved++
	}
	return result, nil
}

func terminal(status Status) bool {
	return status == StatusSucceeded || status == StatusFailed || status == StatusCancelled
}

func (memory *Memory) Claim(ctx context.Context, command ClaimRequest) (ClaimResult, error) {
	if err := ctx.Err(); err != nil {
		return ClaimResult{}, err
	}
	if _, ok := memory.catalog.queues[command.Queue]; !ok {
		return ClaimResult{}, invalid("queue", "is not registered")
	}
	workerID := strings.TrimSpace(command.WorkerID)
	if workerID == "" {
		return ClaimResult{}, invalid("worker_id", "is required")
	}
	if len(workerID) > 512 {
		return ClaimResult{}, invalid("worker_id", "exceeds 512 bytes")
	}
	if err := memory.catalog.validateLease(command.LeaseDuration); err != nil {
		return ClaimResult{}, err
	}
	now := memory.clock().UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	memory.reclaimExpiredLocked(now)
	candidates := make([]*memoryJob, 0)
	for _, value := range memory.jobs {
		if value.job.Queue != command.Queue ||
			(value.job.Status != StatusQueued && value.job.Status != StatusRetrying) ||
			value.job.RunAt.After(now) {
			continue
		}
		candidates = append(candidates, value)
	}
	slices.SortFunc(candidates, func(a, b *memoryJob) int {
		if a.job.Priority != b.job.Priority {
			return b.job.Priority - a.job.Priority
		}
		if order := a.job.RunAt.Compare(b.job.RunAt); order != 0 {
			return order
		}
		if order := a.job.CreatedAt.Compare(b.job.CreatedAt); order != 0 {
			return order
		}
		return strings.Compare(string(a.job.ID), string(b.job.ID))
	})
	if len(candidates) == 0 {
		return ClaimResult{}, nil
	}
	value := candidates[0]
	token, err := NewJobID()
	if err != nil {
		return ClaimResult{}, err
	}
	expires := now.Add(command.LeaseDuration)
	value.job.Status = StatusRunning
	value.job.Attempt++
	value.job.LeaseOwner = workerID
	value.job.LeaseExpiresAt = timePtr(expires)
	value.job.UpdatedAt = now
	if value.job.StartedAt == nil {
		value.job.StartedAt = timePtr(now)
	}
	value.leaseToken = string(token)
	memory.attempts[value.job.ID] = append(memory.attempts[value.job.ID], Attempt{
		JobID: value.job.ID, Number: value.job.Attempt, WorkerID: workerID,
		Outcome: AttemptRunning, StartedAt: now,
	})
	return ClaimResult{Found: true, Lease: Lease{
		Job: cloneJob(value.job), Token: value.leaseToken, ExpiresAt: expires,
	}}, nil
}

func (memory *Memory) reclaimExpiredLocked(now time.Time) {
	for _, value := range memory.jobs {
		if value.job.Status != StatusRunning || value.job.LeaseExpiresAt == nil || value.job.LeaseExpiresAt.After(now) {
			continue
		}
		memory.finishAttemptLocked(value, AttemptLeaseExpired, "lease expired", now)
		value.job.LastError = "lease expired"
		if value.job.Attempt >= value.job.MaxAttempts {
			value.job.Status = StatusFailed
			value.job.FinishedAt = timePtr(now)
		} else {
			value.job.Status = StatusRetrying
			value.job.RunAt = now.Add(memory.catalog.Backoff(value.job.ID, value.job.Attempt))
		}
		value.job.UpdatedAt = now
		memory.clearLeaseLocked(value)
	}
}

func (memory *Memory) Heartbeat(ctx context.Context, lease Lease, duration time.Duration) (Lease, error) {
	if err := ctx.Err(); err != nil {
		return Lease{}, err
	}
	if err := memory.catalog.validateLease(duration); err != nil {
		return Lease{}, err
	}
	now := memory.clock().UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	value, err := memory.leaseLocked(lease, now)
	if err != nil {
		return Lease{}, err
	}
	expires := now.Add(duration)
	value.job.LeaseExpiresAt = timePtr(expires)
	value.job.UpdatedAt = now
	return Lease{Job: cloneJob(value.job), Token: value.leaseToken, ExpiresAt: expires}, nil
}

func (memory *Memory) ReportProgress(ctx context.Context, lease Lease, progress json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	normalized, err := normalizeJSON(progress, memory.catalog.limits.MaxProgressBytes, "progress")
	if err != nil {
		return err
	}
	now := memory.clock().UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	value, err := memory.leaseLocked(lease, now)
	if err != nil {
		return err
	}
	value.job.Progress = normalized
	value.job.UpdatedAt = now
	return nil
}

func (memory *Memory) Complete(ctx context.Context, command CompleteCommand) (Job, error) {
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	result, err := memory.catalog.validateResult(command.Result)
	if err != nil {
		return Job{}, err
	}
	now := memory.clock().UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	value, err := memory.leaseLocked(command.Lease, now)
	if err != nil {
		return Job{}, err
	}
	value.job.Status = StatusSucceeded
	value.job.ResultSummary = result.Summary
	value.job.Result = cloneJSON(result.Data)
	value.job.LastError = ""
	value.job.FinishedAt = timePtr(now)
	value.job.UpdatedAt = now
	memory.finishAttemptLocked(value, AttemptSucceeded, "", now)
	memory.clearLeaseLocked(value)
	return cloneJob(value.job), nil
}

func (memory *Memory) Retry(ctx context.Context, command RetryCommand) (Job, error) {
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	if strings.TrimSpace(command.Error) == "" {
		return Job{}, invalid("error", "is required")
	}
	now := memory.clock().UTC()
	runAt := command.RunAt.UTC()
	if command.RunAt.IsZero() || runAt.Before(now) {
		runAt = now
	}
	if runAt.After(now.Add(memory.catalog.limits.MaxDelay)) {
		return Job{}, invalid("run_at", "exceeds maximum delay")
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	value, err := memory.leaseLocked(command.Lease, now)
	if err != nil {
		return Job{}, err
	}
	if value.job.Attempt >= value.job.MaxAttempts {
		return Job{}, conflict("attempt", "attempt budget is exhausted")
	}
	value.job.Status = StatusRetrying
	value.job.LastError = command.Error
	value.job.RunAt = runAt
	value.job.UpdatedAt = now
	memory.finishAttemptLocked(value, AttemptRetrying, command.Error, now)
	memory.clearLeaseLocked(value)
	return cloneJob(value.job), nil
}

func (memory *Memory) Fail(ctx context.Context, command FailCommand) (Job, error) {
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	if strings.TrimSpace(command.Error) == "" {
		return Job{}, invalid("error", "is required")
	}
	now := memory.clock().UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	value, err := memory.leaseLocked(command.Lease, now)
	if err != nil {
		return Job{}, err
	}
	value.job.Status = StatusFailed
	value.job.LastError = command.Error
	value.job.FinishedAt = timePtr(now)
	value.job.UpdatedAt = now
	memory.finishAttemptLocked(value, AttemptFailed, command.Error, now)
	memory.clearLeaseLocked(value)
	return cloneJob(value.job), nil
}

func (memory *Memory) leaseLocked(lease Lease, now time.Time) (*memoryJob, error) {
	value, ok := memory.jobs[lease.Job.ID]
	if !ok {
		return nil, leaseLost("job does not exist")
	}
	if value.job.Status != StatusRunning || value.leaseToken == "" ||
		value.leaseToken != lease.Token {
		return nil, leaseLost("ownership changed")
	}
	if value.job.LeaseExpiresAt == nil || !value.job.LeaseExpiresAt.After(now) {
		return nil, leaseLost("expired")
	}
	return value, nil
}

func (memory *Memory) finishAttemptLocked(value *memoryJob, outcome AttemptOutcome, message string, now time.Time) {
	items := memory.attempts[value.job.ID]
	if len(items) == 0 || items[len(items)-1].Outcome != AttemptRunning {
		return
	}
	items[len(items)-1].Outcome = outcome
	items[len(items)-1].Error = message
	items[len(items)-1].FinishedAt = timePtr(now)
	memory.attempts[value.job.ID] = items
}

func (memory *Memory) clearLeaseLocked(value *memoryJob) {
	value.leaseToken = ""
	value.job.LeaseOwner = ""
	value.job.LeaseExpiresAt = nil
}

func (memory *Memory) MaterializeSchedules(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	now := memory.clock().UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	created := 0
	keys := make([]string, 0, len(memory.scheduleCursor))
	for key := range memory.scheduleCursor {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		next := memory.scheduleCursor[key]
		for !next.After(now) && created < memory.catalog.limits.MaxScheduleCatchUp {
			request, ok := memory.catalog.scheduleRequest(key, next)
			if !ok {
				break
			}
			prepared, err := memory.catalog.Prepare(now, request)
			if err != nil {
				return created, err
			}
			result, err := memory.enqueueLocked(now, prepared, "")
			if err != nil {
				return created, err
			}
			if !result.Replay {
				created++
			}
			following, ok := memory.catalog.nextSchedule(key, next)
			if !ok {
				break
			}
			next = following
			memory.scheduleCursor[key] = next
		}
	}
	return created, nil
}

func cloneJob(value Job) Job {
	value.Payload = cloneJSON(value.Payload)
	value.Metadata = cloneJSON(value.Metadata)
	value.Progress = cloneJSON(value.Progress)
	value.Result = cloneJSON(value.Result)
	if value.StartedAt != nil {
		value.StartedAt = timePtr(*value.StartedAt)
	}
	if value.FinishedAt != nil {
		value.FinishedAt = timePtr(*value.FinishedAt)
	}
	if value.CancelledAt != nil {
		value.CancelledAt = timePtr(*value.CancelledAt)
	}
	if value.LeaseExpiresAt != nil {
		value.LeaseExpiresAt = timePtr(*value.LeaseExpiresAt)
	}
	return value
}

func timePtr(value time.Time) *time.Time {
	return &value
}
