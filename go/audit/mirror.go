package audit

import (
	"context"
	"log/slog"
	"time"
)

type CommittedEvent struct {
	Event       Event     `json:"event"`
	CommittedAt time.Time `json:"committedAt"`
}

type Mirror interface {
	Publish(context.Context, []CommittedEvent) error
}

type SlogMirror struct {
	Logger *slog.Logger
	Level  slog.Level
}

func (mirror SlogMirror) Publish(ctx context.Context, events []CommittedEvent) error {
	logger := mirror.Logger
	if logger == nil {
		logger = slog.Default()
	}
	level := mirror.Level
	if level == 0 {
		level = slog.LevelInfo
	}
	for _, committed := range events {
		event := committed.Event
		logger.LogAttrs(ctx, level, "audit event committed",
			slog.String("audit.event_id", string(event.ID)),
			slog.Uint64("audit.sequence", uint64(event.Sequence)),
			slog.String("audit.action", string(event.Action.Name)),
			slog.Int("audit.action_version", int(event.Action.Version)),
			slog.String("audit.actor_kind", string(event.Actor.Kind)),
			slog.String("audit.actor_id", event.Actor.ID),
			slog.String("audit.target_type", event.Target.Type),
			slog.String("audit.target_id", event.Target.ID),
			slog.String("audit.outcome", string(event.Outcome.Kind)),
			slog.String("audit.reason", string(event.Outcome.Reason)),
			slog.String("audit.request_id", event.Correlation.RequestID),
			slog.String("audit.trace_id", event.Correlation.TraceID),
			slog.String("audit.digest", string(event.Digest)),
			slog.Time("audit.occurred_at", event.OccurredAt),
			slog.Time("audit.recorded_at", event.RecordedAt),
			slog.Time("audit.committed_at", committed.CommittedAt),
		)
	}
	return nil
}

type MirrorDispatchOptions struct {
	BatchSize int
	Lease     time.Duration
	Retry     time.Duration
	Clock     Clock
}

type MirrorDispatchResult struct {
	Selected  int `json:"selected"`
	Delivered int `json:"delivered"`
	Failed    int `json:"failed"`
}

type MirrorBacklog struct {
	Pending       uint64     `json:"pending"`
	OldestPending *time.Time `json:"oldestPending,omitempty"`
}
