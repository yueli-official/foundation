package audit

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"slices"
	"time"
)

type Query struct {
	Actions          []Action
	Actor            *Actor
	Target           *Target
	Outcomes         []OutcomeKind
	RetentionClasses []RetentionClass
	RequestID        string
	TraceID          string
	CommandID        string
	From             time.Time
	To               time.Time
	Before           Cursor
	Limit            int
}

type Page struct {
	Events     []Event
	NextCursor Cursor
}

type cursorPayload struct {
	Version      uint16   `json:"version"`
	Before       Sequence `json:"before"`
	FilterDigest string   `json:"filterDigest"`
}

func normalizeQuery(query Query) (Query, string, error) {
	query.Actions = slices.Clone(query.Actions)
	query.Outcomes = slices.Clone(query.Outcomes)
	query.RetentionClasses = slices.Clone(query.RetentionClasses)
	if query.Limit == 0 {
		query.Limit = 100
	}
	if query.Limit < 1 || query.Limit > 500 {
		return Query{}, "", &Error{Kind: ErrorInvalidCursor, Field: "limit", Message: "must be between 1 and 500"}
	}
	if !query.From.IsZero() {
		query.From = query.From.UTC()
	}
	if !query.To.IsZero() {
		query.To = query.To.UTC()
	}
	if !query.From.IsZero() && !query.To.IsZero() && !query.From.Before(query.To) {
		return Query{}, "", &Error{Kind: ErrorInvalidCursor, Field: "time", Message: "from must be before to"}
	}
	slices.SortFunc(query.Actions, func(a, b Action) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return int(a.Version) - int(b.Version)
	})
	slices.Sort(query.Outcomes)
	slices.Sort(query.RetentionClasses)
	raw, err := json.Marshal(struct {
		Actions          []Action
		Actor            *Actor
		Target           *Target
		Outcomes         []OutcomeKind
		RetentionClasses []RetentionClass
		RequestID        string
		TraceID          string
		CommandID        string
		From             time.Time
		To               time.Time
	}{
		query.Actions, query.Actor, query.Target, query.Outcomes, query.RetentionClasses,
		query.RequestID, query.TraceID, query.CommandID, query.From, query.To,
	})
	if err != nil {
		return Query{}, "", &Error{Kind: ErrorInvalidCursor, Field: "query", Message: "cannot be encoded"}
	}
	sum := sha256.Sum256(raw)
	return query, hex.EncodeToString(sum[:]), nil
}

func encodeCursor(before Sequence, filterDigest string) Cursor {
	raw, _ := json.Marshal(cursorPayload{Version: 1, Before: before, FilterDigest: filterDigest})
	return Cursor(base64.RawURLEncoding.EncodeToString(raw))
}

func decodeCursor(value Cursor, filterDigest string) (Sequence, error) {
	if value == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(string(value))
	if err != nil {
		return 0, &Error{Kind: ErrorInvalidCursor, Field: "before", Message: "is invalid"}
	}
	var payload cursorPayload
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Version != 1 || payload.Before == 0 || payload.FilterDigest != filterDigest {
		return 0, &Error{Kind: ErrorInvalidCursor, Field: "before", Message: "does not match this query"}
	}
	return payload.Before, nil
}

func matchesQuery(event Event, query Query) bool {
	if len(query.Actions) > 0 && !slices.Contains(query.Actions, event.Action) {
		return false
	}
	if query.Actor != nil && *query.Actor != event.Actor {
		return false
	}
	if query.Target != nil && *query.Target != event.Target {
		return false
	}
	if len(query.Outcomes) > 0 && !slices.Contains(query.Outcomes, event.Outcome.Kind) {
		return false
	}
	if len(query.RetentionClasses) > 0 && !slices.Contains(query.RetentionClasses, event.RetentionClass) {
		return false
	}
	if query.RequestID != "" && query.RequestID != event.Correlation.RequestID {
		return false
	}
	if query.TraceID != "" && query.TraceID != event.Correlation.TraceID {
		return false
	}
	if query.CommandID != "" && query.CommandID != event.Correlation.CommandID {
		return false
	}
	if !query.From.IsZero() && event.OccurredAt.Before(query.From) {
		return false
	}
	if !query.To.IsZero() && !event.OccurredAt.Before(query.To) {
		return false
	}
	return true
}
