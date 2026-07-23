package traffic

import (
	"context"
	"time"
)

type Recorder interface {
	Record(context.Context, Observation) (RecordResult, error)
	// RecordBatch is atomic: all observations validate and commit, or none do.
	// Results preserve input order. Repeated EventIDs with the same payload are
	// replays; the same EventID with a different payload conflicts.
	RecordBatch(context.Context, []Observation) ([]RecordResult, error)
}

type VisitorTokenizer interface {
	// TokenizeVisitor derives a daily, instance-scoped token from a caller-owned
	// ephemeral seed. The seed is never persisted by the Module.
	TokenizeVisitor(context.Context, time.Time, []byte) (VisitorToken, error)
}

type Reader interface {
	Summary(context.Context, SummaryQuery) (Summary, error)
	Series(context.Context, SeriesQuery) ([]SeriesPoint, error)
	Top(context.Context, TopQuery) ([]TopEntry, error)
	Totals(context.Context, []Resource) ([]ResourceTotals, error)
}

type Importer interface {
	ImportBaseline(context.Context, BaselineImport) (ImportResult, error)
}

type Maintainer interface {
	Prune(context.Context, time.Time) (PruneResult, error)
	// ForgetResource removes resource-scoped data but deliberately preserves
	// instance aggregates, whose unique counts cannot be safely subtracted.
	ForgetResource(context.Context, Resource) (ForgetResult, error)
}

type Module interface {
	Recorder
	VisitorTokenizer
	Reader
	Importer
	Maintainer
}
