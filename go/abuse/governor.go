package abuse

import (
	"context"
	"slices"
	"strings"
)

type governor struct {
	runtime *runtime
}

func (governor *governor) Actions(ctx context.Context) ([]ActionView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if governor == nil || governor.runtime == nil || governor.runtime.catalog == nil {
		return nil, &Error{Kind: ErrorStoreUnavailable, Field: "governor", Message: "is not initialized"}
	}
	result := make([]ActionView, 0, len(governor.runtime.catalog.actions))
	for _, action := range governor.runtime.catalog.actions {
		result = append(result, ActionView{
			Key: action.def.Key, RequiredSlots: append([]SlotKey(nil), action.requiredSlots...),
			ResolutionRequired: action.def.Resolution != nil,
		})
	}
	slices.SortFunc(result, func(a, b ActionView) int {
		return strings.Compare(string(a.Key), string(b.Key))
	})
	return result, nil
}

func (governor *governor) Inspect(ctx context.Context, query InspectQuery) (Inspection, error) {
	action, err := governor.runtime.Action(query.Action)
	if err != nil {
		return Inspection{}, err
	}
	bound := action.(*boundAction)
	prepared, err := governor.runtime.prepare(bound.action, Input{
		ID: "governance-inspect", Signals: query.Signals,
	})
	if err != nil {
		return Inspection{}, err
	}
	return governor.runtime.store.inspect(ctx, prepared)
}

func (governor *governor) Reset(ctx context.Context, command ResetCommand) (ResetResult, error) {
	if strings.TrimSpace(command.Reason) == "" {
		return ResetResult{}, invalidInput("reason", "is required")
	}
	action, err := governor.runtime.Action(command.Action)
	if err != nil {
		return ResetResult{}, err
	}
	bound := action.(*boundAction)
	prepared, err := governor.runtime.prepare(bound.action, Input{
		ID: "governance-reset", Signals: command.Signals,
	})
	if err != nil {
		return ResetResult{}, err
	}
	return governor.runtime.store.reset(ctx, prepared)
}

func (governor *governor) Prune(ctx context.Context, command PruneCommand) (PruneResult, error) {
	if command.Before.IsZero() {
		return PruneResult{}, invalidInput("before", "is required")
	}
	if command.Limit == 0 {
		command.Limit = 1000
	}
	if command.Limit < 1 || command.Limit > 10000 {
		return PruneResult{}, invalidInput("limit", "must be between 1 and 10000")
	}
	return governor.runtime.store.prune(ctx, command)
}
