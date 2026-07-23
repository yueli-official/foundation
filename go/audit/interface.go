package audit

import (
	"context"
	"io"
)

// Context aliases context.Context only to keep generic helpers readable while
// preserving the ordinary Go interface.
type Context = context.Context

type Appender interface {
	Append(context.Context, Command) (Event, error)
	AppendBatch(context.Context, []Command) ([]Event, error)
}

type Reader interface {
	Query(context.Context, Query) (Page, error)
	Get(context.Context, EventID) (Event, bool, error)
}

type Exporter interface {
	Export(context.Context, ExportRequest, io.Writer) (ExportManifest, error)
}

type Verifier interface {
	Verify(context.Context, VerifyRequest) (VerifyResult, error)
}

type Module interface {
	Appender
	Reader
	Exporter
	Maintainer
}
