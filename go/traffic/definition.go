package traffic

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"time"
)

const DefinitionVersion uint64 = 1

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

type ResourceKindDefinition struct {
	Key ResourceKind `json:"key"`
}

type CollectionPolicy struct {
	CountedClasses []VisitClass `json:"countedClasses,omitempty"`
}

type Limits struct {
	MaxEventIDBytes        int           `json:"maxEventIdBytes,omitempty"`
	MaxResourceIDBytes     int           `json:"maxResourceIdBytes,omitempty"`
	MaxBaselineSourceBytes int           `json:"maxBaselineSourceBytes,omitempty"`
	MaxBatchSize           int           `json:"maxBatchSize,omitempty"`
	MaxQueryDays           int           `json:"maxQueryDays,omitempty"`
	MaxPastAge             time.Duration `json:"maxPastAge,omitempty"`
	MaxFutureSkew          time.Duration `json:"maxFutureSkew,omitempty"`
	ReceiptRetention       time.Duration `json:"receiptRetention,omitempty"`
	VisitorMarkerRetention time.Duration `json:"visitorMarkerRetention,omitempty"`
}

type Definition struct {
	Version       uint64                   `json:"version"`
	TimeZone      string                   `json:"timeZone,omitempty"`
	ResourceKinds []ResourceKindDefinition `json:"resourceKinds"`
	Policy        CollectionPolicy         `json:"policy,omitempty"`
	Limits        Limits                   `json:"limits,omitempty"`
}

type Catalog struct {
	version       uint64
	timeZone      string
	location      *time.Location
	resourceKinds map[ResourceKind]struct{}
	counted       map[VisitClass]struct{}
	limits        Limits
	digest        string
}

func Compile(definition Definition) (*Catalog, error) {
	if definition.Version != DefinitionVersion {
		return nil, invalid("version", "must equal %d", DefinitionVersion)
	}
	timeZone := strings.TrimSpace(definition.TimeZone)
	if timeZone == "" {
		timeZone = "UTC"
	}
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		return nil, invalid("time_zone", "must be a valid IANA time zone")
	}
	if len(definition.ResourceKinds) == 0 {
		return nil, invalid("resource_kinds", "must contain at least one kind")
	}
	kinds := make(map[ResourceKind]struct{}, len(definition.ResourceKinds))
	canonicalKinds := make([]string, 0, len(definition.ResourceKinds))
	for index, item := range definition.ResourceKinds {
		key := ResourceKind(strings.TrimSpace(string(item.Key)))
		if !keyPattern.MatchString(string(key)) {
			return nil, invalid("resource_kinds", "item %d has an invalid key", index)
		}
		if _, exists := kinds[key]; exists {
			return nil, invalid("resource_kinds", "contains duplicate %q", key)
		}
		kinds[key] = struct{}{}
		canonicalKinds = append(canonicalKinds, string(key))
	}
	slices.Sort(canonicalKinds)

	classes := definition.Policy.CountedClasses
	if len(classes) == 0 {
		classes = []VisitClass{VisitUnknown, VisitHuman}
	}
	counted := make(map[VisitClass]struct{}, len(classes))
	canonicalClasses := make([]string, 0, len(classes))
	for index, class := range classes {
		if !validVisitClass(class) {
			return nil, invalid("policy.counted_classes", "item %d is unknown", index)
		}
		if _, exists := counted[class]; exists {
			return nil, invalid("policy.counted_classes", "contains duplicate %q", class)
		}
		counted[class] = struct{}{}
		canonicalClasses = append(canonicalClasses, string(class))
	}
	slices.Sort(canonicalClasses)

	limits, err := normalizeLimits(definition.Limits)
	if err != nil {
		return nil, err
	}
	canonical := struct {
		Version  uint64   `json:"version"`
		TimeZone string   `json:"timeZone"`
		Kinds    []string `json:"resourceKinds"`
		Classes  []string `json:"countedClasses"`
		Limits   Limits   `json:"limits"`
	}{
		Version: definition.Version, TimeZone: timeZone, Kinds: canonicalKinds,
		Classes: canonicalClasses, Limits: limits,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return nil, &Error{Kind: ErrorUnavailable, Field: "definition", Message: "cannot encode canonical definition", Cause: err}
	}
	sum := sha256.Sum256(encoded)
	return &Catalog{
		version: definition.Version, timeZone: timeZone, location: location,
		resourceKinds: kinds, counted: counted, limits: limits,
		digest: hex.EncodeToString(sum[:]),
	}, nil
}

func MustCompile(definition Definition) *Catalog {
	catalog, err := Compile(definition)
	if err != nil {
		panic(err)
	}
	return catalog
}

func normalizeLimits(value Limits) (Limits, error) {
	if value.MaxEventIDBytes == 0 {
		value.MaxEventIDBytes = 200
	}
	if value.MaxResourceIDBytes == 0 {
		value.MaxResourceIDBytes = 200
	}
	if value.MaxBaselineSourceBytes == 0 {
		value.MaxBaselineSourceBytes = 100
	}
	if value.MaxBatchSize == 0 {
		value.MaxBatchSize = 100
	}
	if value.MaxQueryDays == 0 {
		value.MaxQueryDays = 3660
	}
	if value.MaxPastAge == 0 {
		value.MaxPastAge = 7 * 24 * time.Hour
	}
	if value.MaxFutureSkew == 0 {
		value.MaxFutureSkew = 5 * time.Minute
	}
	if value.ReceiptRetention == 0 {
		value.ReceiptRetention = 7 * 24 * time.Hour
	}
	if value.VisitorMarkerRetention == 0 {
		value.VisitorMarkerRetention = 9 * 24 * time.Hour
	}
	if value.MaxEventIDBytes < 16 || value.MaxEventIDBytes > 1024 {
		return Limits{}, invalid("limits.max_event_id_bytes", "must be between 16 and 1024")
	}
	if value.MaxResourceIDBytes < 1 || value.MaxResourceIDBytes > 1024 {
		return Limits{}, invalid("limits.max_resource_id_bytes", "must be between 1 and 1024")
	}
	if value.MaxBaselineSourceBytes < 1 || value.MaxBaselineSourceBytes > 256 {
		return Limits{}, invalid("limits.max_baseline_source_bytes", "must be between 1 and 256")
	}
	if value.MaxBatchSize < 1 || value.MaxBatchSize > 1000 {
		return Limits{}, invalid("limits.max_batch_size", "must be between 1 and 1000")
	}
	if value.MaxQueryDays < 1 || value.MaxQueryDays > 36525 {
		return Limits{}, invalid("limits.max_query_days", "must be between 1 and 36525")
	}
	if value.MaxPastAge <= 0 {
		return Limits{}, invalid("limits.max_past_age", "must be positive")
	}
	if value.MaxFutureSkew < 0 || value.MaxFutureSkew > 24*time.Hour {
		return Limits{}, invalid("limits.max_future_skew", "must be between zero and 24 hours")
	}
	if value.ReceiptRetention < value.MaxPastAge {
		return Limits{}, invalid("limits.receipt_retention", "must be at least max_past_age")
	}
	if value.VisitorMarkerRetention < value.MaxPastAge+24*time.Hour {
		return Limits{}, invalid("limits.visitor_marker_retention", "must be at least max_past_age plus 24 hours")
	}
	return value, nil
}

func validVisitClass(class VisitClass) bool {
	switch class {
	case VisitUnknown, VisitHuman, VisitBot, VisitInternal:
		return true
	default:
		return false
	}
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

func (catalog *Catalog) TimeZone() string {
	if catalog == nil {
		return ""
	}
	return catalog.timeZone
}

func (catalog *Catalog) Limits() Limits {
	if catalog == nil {
		return Limits{}
	}
	return catalog.limits
}

func (catalog *Catalog) ResourceKinds() []ResourceKind {
	if catalog == nil {
		return nil
	}
	result := make([]ResourceKind, 0, len(catalog.resourceKinds))
	for kind := range catalog.resourceKinds {
		result = append(result, kind)
	}
	slices.Sort(result)
	return result
}

func (catalog *Catalog) DayAt(value time.Time) Day {
	if catalog == nil || catalog.location == nil {
		return Day{}
	}
	return dayFromTime(value, catalog.location)
}

func (catalog *Catalog) PrepareObservation(now time.Time, observation Observation) (PreparedObservation, error) {
	if catalog == nil {
		return PreparedObservation{}, invalid("catalog", "is required")
	}
	eventID := EventID(strings.TrimSpace(string(observation.EventID)))
	if eventID == "" {
		return PreparedObservation{}, invalid("event_id", "is required")
	}
	if len(eventID) > catalog.limits.MaxEventIDBytes {
		return PreparedObservation{}, invalid("event_id", "exceeds %d bytes", catalog.limits.MaxEventIDBytes)
	}
	resource, err := catalog.NormalizeResource(observation.Resource)
	if err != nil {
		return PreparedObservation{}, err
	}
	if observation.OccurredAt.IsZero() {
		return PreparedObservation{}, invalid("occurred_at", "is required")
	}
	occurredAt := observation.OccurredAt.UTC()
	now = now.UTC()
	if occurredAt.Before(now.Add(-catalog.limits.MaxPastAge)) {
		return PreparedObservation{}, invalid("occurred_at", "is older than max_past_age")
	}
	if occurredAt.After(now.Add(catalog.limits.MaxFutureSkew)) {
		return PreparedObservation{}, invalid("occurred_at", "is later than max_future_skew")
	}
	class := observation.Class
	if class == "" {
		class = VisitUnknown
	}
	if !validVisitClass(class) {
		return PreparedObservation{}, invalid("class", "is unknown")
	}
	if observation.HasVisitor && observation.VisitorToken.IsZero() {
		return PreparedObservation{}, invalid("visitor_token", "is required when has_visitor is true")
	}
	if !observation.HasVisitor && !observation.VisitorToken.IsZero() {
		return PreparedObservation{}, invalid("has_visitor", "must be true when visitor_token is present")
	}
	prepared := PreparedObservation{
		EventID: eventID, Resource: resource, OccurredAt: occurredAt,
		Day: catalog.DayAt(occurredAt), Class: class,
		HasVisitor: observation.HasVisitor, VisitorToken: observation.VisitorToken,
		Counted: true,
	}
	if _, ok := catalog.counted[class]; !ok {
		prepared.Counted = false
		switch class {
		case VisitBot:
			prepared.DropReason = DropBot
		case VisitInternal:
			prepared.DropReason = DropInternal
		default:
			prepared.DropReason = DropPolicy
		}
	}
	prepared.Fingerprint = observationFingerprint(prepared)
	return prepared, nil
}

func (catalog *Catalog) NormalizeResource(resource Resource) (Resource, error) {
	if catalog == nil {
		return Resource{}, invalid("catalog", "is required")
	}
	resource.Kind = ResourceKind(strings.TrimSpace(string(resource.Kind)))
	resource.ID = strings.TrimSpace(resource.ID)
	if _, ok := catalog.resourceKinds[resource.Kind]; !ok {
		return Resource{}, invalid("resource.kind", "is not registered")
	}
	if resource.ID == "" {
		return Resource{}, invalid("resource.id", "is required")
	}
	if len(resource.ID) > catalog.limits.MaxResourceIDBytes {
		return Resource{}, invalid("resource.id", "exceeds %d bytes", catalog.limits.MaxResourceIDBytes)
	}
	if strings.ContainsRune(resource.ID, '\x00') {
		return Resource{}, invalid("resource.id", "contains NUL")
	}
	return resource, nil
}

func (catalog *Catalog) NormalizeScope(scope Scope) (Scope, error) {
	switch scope.Kind {
	case ScopeInstance:
		if scope.Resource.Kind != "" || scope.Resource.ID != "" {
			return Scope{}, invalid("scope.resource", "must be empty for instance scope")
		}
		return InstanceScope(), nil
	case ScopeResource:
		resource, err := catalog.NormalizeResource(scope.Resource)
		if err != nil {
			return Scope{}, err
		}
		return ResourceScope(resource), nil
	default:
		return Scope{}, invalid("scope.kind", "is unknown")
	}
}

func (catalog *Catalog) NormalizeRange(value DateRange) (DateRange, error) {
	if catalog == nil {
		return DateRange{}, invalid("catalog", "is required")
	}
	if value.From.IsZero() || value.To.IsZero() {
		return DateRange{}, invalid("range", "from and to are required")
	}
	from := value.From.at(catalog.location)
	to := value.To.at(catalog.location)
	if !to.After(from) {
		return DateRange{}, invalid("range", "to must be after from")
	}
	days := 0
	for cursor := value.From; cursor != value.To; cursor = cursor.add(1, catalog.location) {
		days++
		if days > catalog.limits.MaxQueryDays {
			return DateRange{}, invalid("range", "exceeds %d days", catalog.limits.MaxQueryDays)
		}
	}
	return value, nil
}

func (catalog *Catalog) IsCounted(class VisitClass) bool {
	if catalog == nil {
		return false
	}
	if class == "" {
		class = VisitUnknown
	}
	_, ok := catalog.counted[class]
	return ok
}

// DeriveVisitorToken is an Adapter helper. It performs domain-separated HMAC
// over the Catalog day and a caller-owned ephemeral seed. The seed is never
// retained. Production Adapters should use an instance-specific secret.
func (catalog *Catalog) DeriveVisitorToken(secret []byte, at time.Time, seed []byte) (VisitorToken, error) {
	if catalog == nil {
		return VisitorToken{}, invalid("catalog", "is required")
	}
	if len(secret) < 32 {
		return VisitorToken{}, invalid("secret", "must contain at least 32 bytes")
	}
	if len(seed) == 0 {
		return VisitorToken{}, invalid("visitor_seed", "is required")
	}
	if len(seed) > 4096 {
		return VisitorToken{}, invalid("visitor_seed", "exceeds 4096 bytes")
	}
	if at.IsZero() {
		return VisitorToken{}, invalid("at", "is required")
	}
	day := catalog.DayAt(at)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("foundation.traffic.visitor.v1"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(catalog.Digest()))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(day.String()))
	_, _ = mac.Write([]byte{0})
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(seed)))
	_, _ = mac.Write(size[:])
	_, _ = mac.Write(seed)
	var token VisitorToken
	copy(token[:], mac.Sum(nil))
	return token, nil
}

func (catalog *Catalog) dayAdd(day Day, count int) Day {
	return day.add(count, catalog.location)
}

func (catalog *Catalog) dayEnd(day Day) time.Time {
	return day.add(1, catalog.location).at(catalog.location)
}
