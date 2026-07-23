// Package otelmirror projects committed audit events into OpenTelemetry Logs.
// It is an observability mirror only; the audit Journal remains authoritative.
package otelmirror

import (
	"context"

	"go.opentelemetry.io/otel/log"

	"github.com/yueli-official/foundation/go/audit"
)

type Mirror struct {
	Logger log.Logger
}

func (mirror Mirror) Publish(ctx context.Context, events []audit.CommittedEvent) error {
	if mirror.Logger == nil {
		return &audit.Error{Kind: audit.ErrorInvalidAttempt, Field: "logger", Message: "is required"}
	}
	for _, committed := range events {
		event := committed.Event
		var record log.Record
		record.SetEventName(string(event.Action.Name))
		record.SetTimestamp(event.OccurredAt)
		record.SetObservedTimestamp(event.RecordedAt)
		record.SetSeverity(severity(event.Outcome.Kind))
		record.SetSeverityText(string(event.Outcome.Kind))
		record.SetBody(log.StringValue("audit event committed"))
		record.AddAttributes(
			log.String("log.record.uid", string(event.ID)),
			log.Int64("audit.sequence", int64(event.Sequence)),
			log.Int("audit.action.version", int(event.Action.Version)),
			log.String("audit.actor.kind", string(event.Actor.Kind)),
			log.String("audit.actor.id", event.Actor.ID),
			log.String("audit.target.type", event.Target.Type),
			log.String("audit.target.id", event.Target.ID),
			log.String("audit.outcome", string(event.Outcome.Kind)),
			log.String("audit.reason", string(event.Outcome.Reason)),
			log.String("audit.retention_class", string(event.RetentionClass)),
			log.String("audit.digest", string(event.Digest)),
			log.String("request.id", event.Correlation.RequestID),
			log.String("trace.id", event.Correlation.TraceID),
			log.String("span.id", event.Correlation.SpanID),
			log.String("service.name", event.Source.Service),
			log.String("service.instance.id", event.Source.Instance),
			log.String("service.version", event.Source.Version),
		)
		mirror.Logger.Emit(ctx, record)
	}
	return nil
}

func severity(outcome audit.OutcomeKind) log.Severity {
	switch outcome {
	case audit.OutcomeFailed:
		return log.SeverityError
	case audit.OutcomeDenied:
		return log.SeverityWarn
	default:
		return log.SeverityInfo
	}
}
