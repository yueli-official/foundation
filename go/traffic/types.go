package traffic

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ResourceKind string
type EventID string

type Resource struct {
	Kind ResourceKind `json:"kind"`
	ID   string       `json:"id"`
}

type VisitClass string

const (
	VisitUnknown  VisitClass = "unknown"
	VisitHuman    VisitClass = "human"
	VisitBot      VisitClass = "bot"
	VisitInternal VisitClass = "internal"
)

type DropReason string

const (
	DropNone     DropReason = ""
	DropBot      DropReason = "bot"
	DropInternal DropReason = "internal"
	DropPolicy   DropReason = "policy"
)

// VisitorToken is an opaque, daily-scoped visitor value. Callers must never
// place raw IP addresses, User-Agent strings, account IDs, or emails in it.
type VisitorToken [32]byte

func VisitorTokenFromBytes(value []byte) (VisitorToken, error) {
	var token VisitorToken
	if len(value) != len(token) {
		return token, invalid("visitor_token", "must contain exactly %d bytes", len(token))
	}
	copy(token[:], value)
	return token, nil
}

func ParseVisitorToken(value string) (VisitorToken, error) {
	var token VisitorToken
	decoded, err := hex.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return token, invalid("visitor_token", "must be hexadecimal")
	}
	return VisitorTokenFromBytes(decoded)
}

func (token VisitorToken) Hex() string {
	return hex.EncodeToString(token[:])
}

func (token VisitorToken) IsZero() bool {
	return token == VisitorToken{}
}

// Day is a calendar day without a time zone. Its interpretation is always the
// immutable IANA time zone compiled into a Catalog.
type Day struct {
	year  int
	month time.Month
	day   int
}

func ParseDay(value string) (Day, error) {
	parsed, err := time.Parse(time.DateOnly, strings.TrimSpace(value))
	if err != nil {
		return Day{}, invalid("day", "must use YYYY-MM-DD")
	}
	return Day{year: parsed.Year(), month: parsed.Month(), day: parsed.Day()}, nil
}

func MustParseDay(value string) Day {
	day, err := ParseDay(value)
	if err != nil {
		panic(err)
	}
	return day
}

func (day Day) IsZero() bool {
	return day.year == 0
}

func (day Day) String() string {
	if day.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", day.year, int(day.month), day.day)
}

func (day Day) MarshalText() ([]byte, error) {
	if day.IsZero() {
		return nil, invalid("day", "is required")
	}
	return []byte(day.String()), nil
}

func (day *Day) UnmarshalText(value []byte) error {
	parsed, err := ParseDay(string(value))
	if err != nil {
		return err
	}
	*day = parsed
	return nil
}

func (day Day) MarshalJSON() ([]byte, error) {
	if day.IsZero() {
		return []byte(`""`), nil
	}
	return json.Marshal(day.String())
}

func (day *Day) UnmarshalJSON(value []byte) error {
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return invalid("day", "must be a JSON string")
	}
	if text == "" {
		*day = Day{}
		return nil
	}
	return day.UnmarshalText([]byte(text))
}

func (day Day) at(location *time.Location) time.Time {
	return time.Date(day.year, day.month, day.day, 0, 0, 0, 0, location)
}

func dayFromTime(value time.Time, location *time.Location) Day {
	local := value.In(location)
	return Day{year: local.Year(), month: local.Month(), day: local.Day()}
}

func (day Day) add(days int, location *time.Location) Day {
	return dayFromTime(day.at(location).AddDate(0, 0, days), location)
}

type DateRange struct {
	From Day `json:"from"`
	To   Day `json:"to"`
}

type ScopeKind string

const (
	ScopeInstance ScopeKind = "instance"
	ScopeResource ScopeKind = "resource"
)

type Scope struct {
	Kind     ScopeKind `json:"kind"`
	Resource Resource  `json:"resource,omitempty"`
}

func InstanceScope() Scope {
	return Scope{Kind: ScopeInstance}
}

func ResourceScope(resource Resource) Scope {
	return Scope{Kind: ScopeResource, Resource: resource}
}

type Observation struct {
	EventID      EventID      `json:"eventId"`
	Resource     Resource     `json:"resource"`
	OccurredAt   time.Time    `json:"occurredAt"`
	Class        VisitClass   `json:"class,omitempty"`
	HasVisitor   bool         `json:"hasVisitor,omitempty"`
	VisitorToken VisitorToken `json:"-"`
}

// PreparedObservation is the immutable, validated form used by Adapter
// implementations. Consumers normally use Observation.
type PreparedObservation struct {
	EventID      EventID
	Resource     Resource
	OccurredAt   time.Time
	Day          Day
	Class        VisitClass
	HasVisitor   bool
	VisitorToken VisitorToken
	Counted      bool
	DropReason   DropReason
	Fingerprint  [32]byte
}

type Totals struct {
	Views             int64 `json:"views"`
	UniqueVisitorDays int64 `json:"uniqueVisitorDays"`
}

type RecordResult struct {
	EventID              EventID    `json:"eventId"`
	Counted              bool       `json:"counted"`
	Replay               bool       `json:"replay"`
	DropReason           DropReason `json:"dropReason,omitempty"`
	FirstInstanceVisitor bool       `json:"firstInstanceVisitor"`
	FirstResourceVisitor bool       `json:"firstResourceVisitor"`
	InstanceTotals       Totals     `json:"instanceTotals"`
	ResourceTotals       Totals     `json:"resourceTotals"`
}

type SummaryQuery struct {
	Scope Scope      `json:"scope"`
	Range *DateRange `json:"range,omitempty"`
}

type Summary struct {
	Scope  Scope      `json:"scope"`
	Range  *DateRange `json:"range,omitempty"`
	Totals Totals     `json:"totals"`
}

type SeriesQuery struct {
	Scope Scope     `json:"scope"`
	Range DateRange `json:"range"`
}

type SeriesPoint struct {
	Day    Day    `json:"day"`
	Totals Totals `json:"totals"`
}

type RankMetric string

const (
	RankViews             RankMetric = "views"
	RankUniqueVisitorDays RankMetric = "unique_visitor_days"
)

type TopQuery struct {
	ResourceKind ResourceKind `json:"resourceKind"`
	Range        *DateRange   `json:"range,omitempty"`
	Metric       RankMetric   `json:"metric,omitempty"`
	Limit        int          `json:"limit,omitempty"`
}

type TopEntry struct {
	Resource Resource `json:"resource"`
	Totals   Totals   `json:"totals"`
}

type ResourceTotals struct {
	Resource Resource `json:"resource"`
	Totals   Totals   `json:"totals"`
}

type BaselineImport struct {
	Source   string   `json:"source"`
	Resource Resource `json:"resource"`
	Views    int64    `json:"views"`
}

type ImportResult struct {
	Applied        bool   `json:"applied"`
	Replay         bool   `json:"replay"`
	ResourceTotals Totals `json:"resourceTotals"`
}

type PruneResult struct {
	ReceiptsRemoved       int64 `json:"receiptsRemoved"`
	VisitorMarkersRemoved int64 `json:"visitorMarkersRemoved"`
}

type ForgetResult struct {
	TotalsRemoved         int64 `json:"totalsRemoved"`
	DailyRowsRemoved      int64 `json:"dailyRowsRemoved"`
	VisitorMarkersRemoved int64 `json:"visitorMarkersRemoved"`
	ReceiptsRemoved       int64 `json:"receiptsRemoved"`
	BaselinesRemoved      int64 `json:"baselinesRemoved"`
}

func observationFingerprint(prepared PreparedObservation) [32]byte {
	visitor := ""
	if prepared.HasVisitor {
		visitor = prepared.VisitorToken.Hex()
	}
	value := strings.Join([]string{
		string(prepared.EventID),
		string(prepared.Resource.Kind),
		prepared.Resource.ID,
		prepared.OccurredAt.Format(time.RFC3339Nano),
		string(prepared.Class),
		visitor,
	}, "\x00")
	return sha256.Sum256([]byte(value))
}
