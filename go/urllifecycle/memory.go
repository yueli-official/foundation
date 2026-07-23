package urllifecycle

import (
	"context"
	"io"
	"sync"
	"time"
)

type MemoryOptions struct {
	Clock func() time.Time
}

type Memory struct {
	mu      sync.RWMutex
	catalog *Catalog
	clock   func() time.Time
	state   registryState
}

var _ Module = (*Memory)(nil)

func NewMemory(catalog *Catalog, options MemoryOptions) (*Memory, error) {
	if catalog == nil {
		return nil, invalid("catalog", "is required")
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Memory{catalog: catalog, clock: clock, state: emptyState()}, nil
}

func (memory *Memory) Resolve(ctx context.Context, lookup Lookup) (Resolution, error) {
	if err := ctx.Err(); err != nil {
		return Resolution{}, err
	}
	requested, err := memory.catalog.normalizeLookup(lookup)
	if err != nil {
		return Resolution{}, err
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	return resolveState(memory.catalog, memory.state, requested, memory.clock().UTC())
}

func (memory *Memory) Preview(ctx context.Context, changeSet ChangeSet) (Plan, error) {
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	result, err := planTransition(memory.catalog, memory.state, changeSet, memory.clock().UTC())
	return result.plan, err
}

func (memory *Memory) Apply(ctx context.Context, changeSet ChangeSet, options ApplyOptions) (Receipt, error) {
	if err := ctx.Err(); err != nil {
		return Receipt{}, err
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	result, err := planTransition(memory.catalog, memory.state, changeSet, memory.clock().UTC())
	if err != nil {
		return Receipt{}, err
	}
	if options.Guard != nil &&
		(options.Guard.BaseRevision != result.plan.BaseRevision ||
			options.Guard.IntentDigest != result.plan.IntentDigest) {
		return Receipt{}, &Error{Kind: ErrorStaleRevision, Field: "guard", Message: "preview guard no longer matches"}
	}
	if result.replay != nil {
		receipt := *result.replay
		receipt.Replay = true
		return receipt, nil
	}
	memory.state = result.next
	return result.receipt, nil
}

func (memory *Memory) Inspect(ctx context.Context, query InspectQuery) (Inspection, error) {
	if err := ctx.Err(); err != nil {
		return Inspection{}, err
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	return inspectState(memory.catalog, memory.state, query, memory.clock().UTC())
}

func (memory *Memory) List(ctx context.Context, query ListQuery) (InspectionPage, error) {
	if err := ctx.Err(); err != nil {
		return InspectionPage{}, err
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	return listState(memory.catalog, memory.state, query, memory.clock().UTC())
}

func (memory *Memory) History(ctx context.Context, query HistoryQuery) (HistoryPage, error) {
	if err := ctx.Err(); err != nil {
		return HistoryPage{}, err
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	return historyState(memory.state, query, memory.catalog.limits.MaxPageSize), nil
}

func (memory *Memory) Export(ctx context.Context, query ExportQuery, writer io.Writer) (ArchiveManifest, error) {
	if err := ctx.Err(); err != nil {
		return ArchiveManifest{}, err
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	return exportState(memory.catalog, memory.state, query, writer)
}

func (memory *Memory) VerifyArchive(ctx context.Context, reader io.Reader) (ArchiveReport, error) {
	if err := ctx.Err(); err != nil {
		return ArchiveReport{}, err
	}
	return verifyArchive(memory.catalog, reader)
}

func (memory *Memory) Restore(ctx context.Context, command RestoreCommand, reader io.Reader) (RestoreReport, error) {
	if err := ctx.Err(); err != nil {
		return RestoreReport{}, err
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	next, report, err := restoreArchive(memory.catalog, memory.state, command, reader, memory.clock().UTC())
	if err != nil {
		return RestoreReport{}, err
	}
	if !command.DryRun {
		memory.state = next
	}
	return report, nil
}

func (memory *Memory) RebuildProjection(ctx context.Context, command RebuildCommand) (RebuildReport, error) {
	if err := ctx.Err(); err != nil {
		return RebuildReport{}, err
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	return rebuildState(memory.catalog, &memory.state, command)
}
