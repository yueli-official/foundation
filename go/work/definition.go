package work

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const DefinitionVersion uint64 = 1

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,95}$`)

type QueueDefinition struct {
	Key         Queue `json:"key"`
	Concurrency int   `json:"concurrency"`
}

type KindDefinition struct {
	Key             Kind          `json:"key"`
	Queue           Queue         `json:"queue"`
	DefaultAttempts int           `json:"defaultAttempts,omitempty"`
	MaxAttempts     int           `json:"maxAttempts,omitempty"`
	Timeout         time.Duration `json:"timeout,omitempty"`
}

type ScheduleDefinition struct {
	Key      string          `json:"key"`
	Cron     string          `json:"cron"`
	TimeZone string          `json:"timeZone,omitempty"`
	Kind     Kind            `json:"kind"`
	Payload  json.RawMessage `json:"payload,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
	Priority int             `json:"priority,omitempty"`
}

type RetryPolicy struct {
	BaseDelay time.Duration `json:"baseDelay,omitempty"`
	MaxDelay  time.Duration `json:"maxDelay,omitempty"`
	Jitter    float64       `json:"jitter,omitempty"`
}

type Limits struct {
	MaxPayloadBytes       int           `json:"maxPayloadBytes,omitempty"`
	MaxMetadataBytes      int           `json:"maxMetadataBytes,omitempty"`
	MaxProgressBytes      int           `json:"maxProgressBytes,omitempty"`
	MaxResultBytes        int           `json:"maxResultBytes,omitempty"`
	MaxResultSummaryBytes int           `json:"maxResultSummaryBytes,omitempty"`
	MaxIdempotencyBytes   int           `json:"maxIdempotencyBytes,omitempty"`
	MaxDelay              time.Duration `json:"maxDelay,omitempty"`
	MinLease              time.Duration `json:"minLease,omitempty"`
	MaxLease              time.Duration `json:"maxLease,omitempty"`
	MaxScheduleCatchUp    int           `json:"maxScheduleCatchUp,omitempty"`
}

type Definition struct {
	Version   uint64               `json:"version"`
	Queues    []QueueDefinition    `json:"queues"`
	Kinds     []KindDefinition     `json:"kinds"`
	Schedules []ScheduleDefinition `json:"schedules,omitempty"`
	Retry     RetryPolicy          `json:"retry,omitempty"`
	Limits    Limits               `json:"limits,omitempty"`
}

type compiledKind struct {
	def KindDefinition
}

type compiledSchedule struct {
	def      ScheduleDefinition
	location *time.Location
	schedule cron.Schedule
	payload  json.RawMessage
	metadata json.RawMessage
}

type Catalog struct {
	version   uint64
	digest    string
	queues    map[Queue]QueueDefinition
	kinds     map[Kind]compiledKind
	schedules map[string]compiledSchedule
	retry     RetryPolicy
	limits    Limits
}

func Compile(definition Definition) (*Catalog, error) {
	if definition.Version != DefinitionVersion {
		return nil, invalid("version", "must equal %d", DefinitionVersion)
	}
	retry, err := normalizeRetry(definition.Retry)
	if err != nil {
		return nil, err
	}
	limits, err := normalizeLimits(definition.Limits)
	if err != nil {
		return nil, err
	}
	if len(definition.Queues) == 0 {
		return nil, invalid("queues", "must contain at least one queue")
	}
	queues := make(map[Queue]QueueDefinition, len(definition.Queues))
	canonicalQueues := append([]QueueDefinition(nil), definition.Queues...)
	for index, item := range canonicalQueues {
		item.Key = Queue(strings.TrimSpace(string(item.Key)))
		if !keyPattern.MatchString(string(item.Key)) {
			return nil, invalid("queues", "item %d has an invalid key", index)
		}
		if item.Concurrency <= 0 || item.Concurrency > 1024 {
			return nil, invalid("queues", "item %d concurrency must be between 1 and 1024", index)
		}
		if _, exists := queues[item.Key]; exists {
			return nil, invalid("queues", "contains duplicate %q", item.Key)
		}
		queues[item.Key] = item
		canonicalQueues[index] = item
	}
	slices.SortFunc(canonicalQueues, func(a, b QueueDefinition) int {
		return strings.Compare(string(a.Key), string(b.Key))
	})
	if len(definition.Kinds) == 0 {
		return nil, invalid("kinds", "must contain at least one kind")
	}
	kinds := make(map[Kind]compiledKind, len(definition.Kinds))
	canonicalKinds := append([]KindDefinition(nil), definition.Kinds...)
	for index, item := range canonicalKinds {
		item.Key = Kind(strings.TrimSpace(string(item.Key)))
		item.Queue = Queue(strings.TrimSpace(string(item.Queue)))
		if !keyPattern.MatchString(string(item.Key)) {
			return nil, invalid("kinds", "item %d has an invalid key", index)
		}
		if _, ok := queues[item.Queue]; !ok {
			return nil, invalid("kinds", "item %d references unknown queue %q", index, item.Queue)
		}
		if item.DefaultAttempts == 0 {
			item.DefaultAttempts = 5
		}
		if item.MaxAttempts == 0 {
			item.MaxAttempts = max(item.DefaultAttempts, 25)
		}
		if item.DefaultAttempts < 1 || item.DefaultAttempts > item.MaxAttempts || item.MaxAttempts > 1000 {
			return nil, invalid("kinds", "item %d has invalid attempt limits", index)
		}
		if item.Timeout < 0 || item.Timeout > 30*24*time.Hour {
			return nil, invalid("kinds", "item %d timeout is invalid", index)
		}
		if _, exists := kinds[item.Key]; exists {
			return nil, invalid("kinds", "contains duplicate %q", item.Key)
		}
		kinds[item.Key] = compiledKind{def: item}
		canonicalKinds[index] = item
	}
	slices.SortFunc(canonicalKinds, func(a, b KindDefinition) int {
		return strings.Compare(string(a.Key), string(b.Key))
	})

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedules := make(map[string]compiledSchedule, len(definition.Schedules))
	canonicalSchedules := append([]ScheduleDefinition(nil), definition.Schedules...)
	for index, item := range canonicalSchedules {
		item.Key = strings.TrimSpace(item.Key)
		item.Cron = strings.TrimSpace(item.Cron)
		item.TimeZone = strings.TrimSpace(item.TimeZone)
		if item.TimeZone == "" {
			item.TimeZone = "UTC"
		}
		if !keyPattern.MatchString(item.Key) {
			return nil, invalid("schedules", "item %d has an invalid key", index)
		}
		kind, ok := kinds[item.Kind]
		if !ok {
			return nil, invalid("schedules", "item %d references unknown kind %q", index, item.Kind)
		}
		_ = kind
		location, err := time.LoadLocation(item.TimeZone)
		if err != nil {
			return nil, invalid("schedules", "item %d has an invalid IANA time zone", index)
		}
		parsed, err := parser.Parse(item.Cron)
		if err != nil {
			return nil, invalid("schedules", "item %d has an invalid five-field cron expression", index)
		}
		payload, err := normalizeJSON(item.Payload, limits.MaxPayloadBytes, "schedules.payload")
		if err != nil {
			return nil, err
		}
		metadata, err := normalizeJSON(item.Metadata, limits.MaxMetadataBytes, "schedules.metadata")
		if err != nil {
			return nil, err
		}
		if item.Priority < -1000 || item.Priority > 1000 {
			return nil, invalid("schedules", "item %d priority must be between -1000 and 1000", index)
		}
		if _, exists := schedules[item.Key]; exists {
			return nil, invalid("schedules", "contains duplicate %q", item.Key)
		}
		item.Payload, item.Metadata = payload, metadata
		schedules[item.Key] = compiledSchedule{
			def: item, location: location, schedule: parsed,
			payload: payload, metadata: metadata,
		}
		canonicalSchedules[index] = item
	}
	slices.SortFunc(canonicalSchedules, func(a, b ScheduleDefinition) int {
		return strings.Compare(a.Key, b.Key)
	})

	canonical := struct {
		Version   uint64
		Queues    []QueueDefinition
		Kinds     []KindDefinition
		Schedules []ScheduleDefinition
		Retry     RetryPolicy
		Limits    Limits
	}{
		Version: definition.Version, Queues: canonicalQueues, Kinds: canonicalKinds,
		Schedules: canonicalSchedules, Retry: retry, Limits: limits,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return nil, &Error{Kind: ErrorUnavailable, Field: "definition", Message: "cannot encode canonical definition", Cause: err}
	}
	sum := sha256.Sum256(encoded)
	return &Catalog{
		version: definition.Version, digest: hex.EncodeToString(sum[:]),
		queues: queues, kinds: kinds, schedules: schedules, retry: retry, limits: limits,
	}, nil
}

func MustCompile(definition Definition) *Catalog {
	catalog, err := Compile(definition)
	if err != nil {
		panic(err)
	}
	return catalog
}

func normalizeRetry(value RetryPolicy) (RetryPolicy, error) {
	if value.BaseDelay == 0 {
		value.BaseDelay = time.Second
	}
	if value.MaxDelay == 0 {
		value.MaxDelay = 15 * time.Minute
	}
	if value.Jitter == 0 {
		value.Jitter = 0.2
	}
	if value.BaseDelay <= 0 || value.MaxDelay < value.BaseDelay {
		return RetryPolicy{}, invalid("retry", "base and max delays are invalid")
	}
	if value.Jitter < 0 || value.Jitter > 1 {
		return RetryPolicy{}, invalid("retry.jitter", "must be between zero and one")
	}
	return value, nil
}

func normalizeLimits(value Limits) (Limits, error) {
	if value.MaxPayloadBytes == 0 {
		value.MaxPayloadBytes = 256 << 10
	}
	if value.MaxMetadataBytes == 0 {
		value.MaxMetadataBytes = 32 << 10
	}
	if value.MaxProgressBytes == 0 {
		value.MaxProgressBytes = 32 << 10
	}
	if value.MaxResultBytes == 0 {
		value.MaxResultBytes = 256 << 10
	}
	if value.MaxResultSummaryBytes == 0 {
		value.MaxResultSummaryBytes = 2000
	}
	if value.MaxIdempotencyBytes == 0 {
		value.MaxIdempotencyBytes = 200
	}
	if value.MaxDelay == 0 {
		value.MaxDelay = 10 * 365 * 24 * time.Hour
	}
	if value.MinLease == 0 {
		value.MinLease = 5 * time.Second
	}
	if value.MaxLease == 0 {
		value.MaxLease = 30 * time.Minute
	}
	if value.MaxScheduleCatchUp == 0 {
		value.MaxScheduleCatchUp = 100
	}
	if value.MaxPayloadBytes < 2 || value.MaxPayloadBytes > 16<<20 ||
		value.MaxMetadataBytes < 2 || value.MaxMetadataBytes > 1<<20 ||
		value.MaxProgressBytes < 2 || value.MaxProgressBytes > 1<<20 ||
		value.MaxResultBytes < 2 || value.MaxResultBytes > 16<<20 {
		return Limits{}, invalid("limits", "JSON byte limits are outside supported bounds")
	}
	if value.MaxResultSummaryBytes < 1 || value.MaxResultSummaryBytes > 64<<10 {
		return Limits{}, invalid("limits.max_result_summary_bytes", "is outside supported bounds")
	}
	if value.MaxIdempotencyBytes < 16 || value.MaxIdempotencyBytes > 1024 {
		return Limits{}, invalid("limits.max_idempotency_bytes", "must be between 16 and 1024")
	}
	if value.MaxDelay <= 0 || value.MinLease <= 0 || value.MaxLease < value.MinLease {
		return Limits{}, invalid("limits", "delay or lease limits are invalid")
	}
	if value.MaxScheduleCatchUp < 1 || value.MaxScheduleCatchUp > 10000 {
		return Limits{}, invalid("limits.max_schedule_catch_up", "must be between 1 and 10000")
	}
	return value, nil
}

func (catalog *Catalog) Version() uint64 {
	if catalog == nil {
		return 0
	}
	return catalog.version
}

func (catalog *Catalog) Digest() string {
	if catalog == nil {
		return ""
	}
	return catalog.digest
}

func (catalog *Catalog) Limits() Limits {
	if catalog == nil {
		return Limits{}
	}
	return catalog.limits
}

func (catalog *Catalog) Queues() []QueueDefinition {
	if catalog == nil {
		return nil
	}
	result := make([]QueueDefinition, 0, len(catalog.queues))
	for _, item := range catalog.queues {
		result = append(result, item)
	}
	slices.SortFunc(result, func(a, b QueueDefinition) int {
		return strings.Compare(string(a.Key), string(b.Key))
	})
	return result
}

func (catalog *Catalog) Kind(key Kind) (KindDefinition, bool) {
	if catalog == nil {
		return KindDefinition{}, false
	}
	item, ok := catalog.kinds[key]
	return item.def, ok
}

func (catalog *Catalog) Prepare(now time.Time, request Request) (PreparedRequest, error) {
	if catalog == nil {
		return PreparedRequest{}, invalid("catalog", "is required")
	}
	kindKey := Kind(strings.TrimSpace(string(request.Kind)))
	kind, ok := catalog.kinds[kindKey]
	if !ok {
		return PreparedRequest{}, invalid("kind", "is not registered")
	}
	payload, err := normalizeJSON(request.Payload, catalog.limits.MaxPayloadBytes, "payload")
	if err != nil {
		return PreparedRequest{}, err
	}
	metadata, err := normalizeJSON(request.Metadata, catalog.limits.MaxMetadataBytes, "metadata")
	if err != nil {
		return PreparedRequest{}, err
	}
	now = now.UTC()
	runAt := request.RunAt
	fingerprintRunAt := runAt
	if runAt.IsZero() {
		runAt = now
	} else {
		runAt = runAt.UTC()
		fingerprintRunAt = runAt
	}
	if runAt.After(now.Add(catalog.limits.MaxDelay)) {
		return PreparedRequest{}, invalid("run_at", "exceeds maximum delay")
	}
	if request.Priority < -1000 || request.Priority > 1000 {
		return PreparedRequest{}, invalid("priority", "must be between -1000 and 1000")
	}
	attempts := request.MaxAttempts
	if attempts == 0 {
		attempts = kind.def.DefaultAttempts
	}
	if attempts < 1 || attempts > kind.def.MaxAttempts {
		return PreparedRequest{}, invalid("max_attempts", "must be between 1 and %d", kind.def.MaxAttempts)
	}
	idempotency := strings.TrimSpace(request.IdempotencyKey)
	if len(idempotency) > catalog.limits.MaxIdempotencyBytes {
		return PreparedRequest{}, invalid("idempotency_key", "exceeds %d bytes", catalog.limits.MaxIdempotencyBytes)
	}
	if strings.ContainsRune(idempotency, '\x00') {
		return PreparedRequest{}, invalid("idempotency_key", "contains NUL")
	}
	prepared := PreparedRequest{
		Kind: kindKey, Queue: kind.def.Queue, Payload: payload, Metadata: metadata,
		RunAt: runAt, Priority: request.Priority, MaxAttempts: attempts,
		IdempotencyKey: idempotency,
	}
	encoded, err := json.Marshal(struct {
		Kind        Kind
		Queue       Queue
		Payload     json.RawMessage
		Metadata    json.RawMessage
		RunAt       time.Time
		Priority    int
		MaxAttempts int
	}{
		Kind: prepared.Kind, Queue: prepared.Queue, Payload: prepared.Payload,
		Metadata: prepared.Metadata, RunAt: fingerprintRunAt, Priority: prepared.Priority,
		MaxAttempts: prepared.MaxAttempts,
	})
	if err != nil {
		return PreparedRequest{}, &Error{Kind: ErrorUnavailable, Field: "request", Message: "cannot fingerprint request", Cause: err}
	}
	prepared.Fingerprint = sha256.Sum256(encoded)
	return prepared, nil
}

// PrepareReplay links an already prepared request fingerprint to its source.
// Adapters use it so one replay idempotency key cannot silently point at a
// different terminal job with otherwise identical content.
func (catalog *Catalog) PrepareReplay(prepared PreparedRequest, source JobID) (PreparedRequest, error) {
	if catalog == nil {
		return PreparedRequest{}, invalid("catalog", "is required")
	}
	if strings.TrimSpace(string(source)) == "" {
		return PreparedRequest{}, invalid("replay_of", "is required")
	}
	value := make([]byte, 0, len(prepared.Fingerprint)+1+len(source))
	value = append(value, prepared.Fingerprint[:]...)
	value = append(value, 0)
	value = append(value, source...)
	prepared.Fingerprint = sha256.Sum256(value)
	return prepared, nil
}

func normalizeJSON(value json.RawMessage, limit int, field string) (json.RawMessage, error) {
	if len(bytes.TrimSpace(value)) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if len(value) > limit {
		return nil, invalid(field, "exceeds %d bytes", limit)
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, invalid(field, "must be valid JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, invalid(field, "must contain one JSON value")
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return nil, invalid(field, "cannot be canonicalized")
	}
	if len(canonical) > limit {
		return nil, invalid(field, "exceeds %d bytes", limit)
	}
	return canonical, nil
}

func NewJobID() (JobID, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", &Error{Kind: ErrorUnavailable, Field: "job_id", Message: "cannot generate", Cause: err}
	}
	// UUID v4 layout without introducing a public dependency.
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	text := hex.EncodeToString(value[:])
	return JobID(fmt.Sprintf("%s-%s-%s-%s-%s", text[:8], text[8:12], text[12:16], text[16:20], text[20:])), nil
}

func (catalog *Catalog) Backoff(id JobID, attempt int) time.Duration {
	if catalog == nil {
		return 0
	}
	exponent := min(max(attempt-1, 0), 30)
	base := float64(catalog.retry.BaseDelay) * math.Pow(2, float64(exponent))
	if base > float64(catalog.retry.MaxDelay) {
		base = float64(catalog.retry.MaxDelay)
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", id, attempt)))
	fraction := float64(uint16(sum[0])<<8|uint16(sum[1])) / 65535
	factor := 1 - catalog.retry.Jitter + (2 * catalog.retry.Jitter * fraction)
	return time.Duration(base * factor)
}

func (catalog *Catalog) validateLease(duration time.Duration) error {
	if duration < catalog.limits.MinLease || duration > catalog.limits.MaxLease {
		return invalid("lease_duration", "must be between %s and %s", catalog.limits.MinLease, catalog.limits.MaxLease)
	}
	return nil
}

func (catalog *Catalog) validateResult(result Result) (Result, error) {
	if len(result.Summary) > catalog.limits.MaxResultSummaryBytes {
		return Result{}, invalid("result.summary", "exceeds %d bytes", catalog.limits.MaxResultSummaryBytes)
	}
	data, err := normalizeJSON(result.Data, catalog.limits.MaxResultBytes, "result.data")
	if err != nil {
		return Result{}, err
	}
	result.Data = data
	return result, nil
}

// PrepareResult validates and canonicalizes handler output for an Adapter.
func (catalog *Catalog) PrepareResult(result Result) (Result, error) {
	if catalog == nil {
		return Result{}, invalid("catalog", "is required")
	}
	return catalog.validateResult(result)
}

// PrepareProgress validates and canonicalizes progress for an Adapter.
func (catalog *Catalog) PrepareProgress(value json.RawMessage) (json.RawMessage, error) {
	if catalog == nil {
		return nil, invalid("catalog", "is required")
	}
	return normalizeJSON(value, catalog.limits.MaxProgressBytes, "progress")
}

// ValidateLease validates an Adapter lease duration.
func (catalog *Catalog) ValidateLease(duration time.Duration) error {
	if catalog == nil {
		return invalid("catalog", "is required")
	}
	return catalog.validateLease(duration)
}

func (catalog *Catalog) nextSchedule(key string, after time.Time) (time.Time, bool) {
	item, ok := catalog.schedules[key]
	if !ok {
		return time.Time{}, false
	}
	local := after.In(item.location)
	return item.schedule.Next(local).UTC(), true
}

// ScheduleKeys returns stable recurring schedule keys for Adapter bootstrap.
func (catalog *Catalog) ScheduleKeys() []string {
	if catalog == nil {
		return nil
	}
	result := make([]string, 0, len(catalog.schedules))
	for key := range catalog.schedules {
		result = append(result, key)
	}
	slices.Sort(result)
	return result
}

// NextSchedule is an Adapter helper for persisted recurring cursors.
func (catalog *Catalog) NextSchedule(key string, after time.Time) (time.Time, bool) {
	if catalog == nil {
		return time.Time{}, false
	}
	return catalog.nextSchedule(key, after)
}

// ScheduleRequest is an Adapter helper for exact occurrence materialization.
func (catalog *Catalog) ScheduleRequest(key string, occurrence time.Time) (Request, bool) {
	if catalog == nil {
		return Request{}, false
	}
	return catalog.scheduleRequest(key, occurrence)
}

func (catalog *Catalog) scheduleRequest(key string, occurrence time.Time) (Request, bool) {
	item, ok := catalog.schedules[key]
	if !ok {
		return Request{}, false
	}
	return Request{
		Kind: item.def.Kind, Payload: cloneJSON(item.payload), Metadata: cloneJSON(item.metadata),
		RunAt: occurrence, Priority: item.def.Priority,
		IdempotencyKey: fmt.Sprintf("schedule:%s:%d", key, occurrence.Unix()),
	}, true
}

func cloneJSON(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}
