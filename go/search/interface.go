package search

import (
	"context"
	"time"
)

type Projector interface {
	Apply(context.Context, Batch) (ApplyResult, error)
}

type Index interface {
	Projector
	Search(context.Context, Query) (Page, error)
}

type Rebuilder interface {
	Start(context.Context, StartRebuild) (RebuildState, error)
	Stage(context.Context, RebuildBatch) (RebuildState, error)
	Finish(context.Context, FinishRebuild) (RebuildState, error)
	Status(context.Context, GenerationID) (RebuildState, error)
	Abandon(context.Context, GenerationID) error
	Prune(context.Context, time.Time) (int64, error)
}

type Module interface {
	Index
	Rebuilder
}
