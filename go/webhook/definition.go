package webhook

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

const DefinitionVersion uint64 = 1

var stableKey = regexp.MustCompile(`^[a-z][a-z0-9._:-]{0,199}$`)

type EventTypeDefinition struct {
	Type         EventType `json:"type"`
	SchemaURI    string    `json:"schemaUri,omitempty"`
	SchemaDigest string    `json:"schemaDigest,omitempty"`
	MaxDataBytes int       `json:"maxDataBytes,omitempty"`
}

type InboundSourceDefinition struct {
	Key             InboundSource `json:"key"`
	ExpectedSource  string        `json:"expectedSource"`
	AllowedTypes    []EventType   `json:"allowedTypes"`
	Secret          SecretRef     `json:"secret"`
	TimestampWindow time.Duration `json:"timestampWindow,omitempty"`
	MaxBodyBytes    int           `json:"maxBodyBytes,omitempty"`
}

type RetryPolicy struct {
	MaxAttempts    int           `json:"maxAttempts,omitempty"`
	MaxAge         time.Duration `json:"maxAge,omitempty"`
	BaseDelay      time.Duration `json:"baseDelay,omitempty"`
	MaxDelay       time.Duration `json:"maxDelay,omitempty"`
	RequestTimeout time.Duration `json:"requestTimeout,omitempty"`
	MaxRetryAfter  time.Duration `json:"maxRetryAfter,omitempty"`
}

type Limits struct {
	MaxEventBytes          int `json:"maxEventBytes,omitempty"`
	MaxEndpoints           int `json:"maxEndpoints,omitempty"`
	MaxSubscriptions       int `json:"maxSubscriptions,omitempty"`
	MaxEventTypesPerSub    int `json:"maxEventTypesPerSubscription,omitempty"`
	MaxFanout              int `json:"maxFanout,omitempty"`
	MaxResponseBytes       int `json:"maxResponseBytes,omitempty"`
	MaxDescriptionBytes    int `json:"maxDescriptionBytes,omitempty"`
	MaxIdempotencyKeyBytes int `json:"maxIdempotencyKeyBytes,omitempty"`
}

type RetentionPolicy struct {
	Events          time.Duration `json:"events,omitempty"`
	Attempts        time.Duration `json:"attempts,omitempty"`
	InboundReceipts time.Duration `json:"inboundReceipts,omitempty"`
}

type Definition struct {
	Version        uint64                    `json:"version"`
	Consumer       string                    `json:"consumer"`
	Source         string                    `json:"source"`
	EventTypes     []EventTypeDefinition     `json:"eventTypes"`
	InboundSources []InboundSourceDefinition `json:"inboundSources,omitempty"`
	Retry          RetryPolicy               `json:"retry,omitempty"`
	Limits         Limits                    `json:"limits,omitempty"`
	Retention      RetentionPolicy           `json:"retention,omitempty"`
}

type Catalog struct {
	version        uint64
	consumer       string
	source         string
	digest         string
	eventTypes     map[EventType]EventTypeDefinition
	inboundSources map[InboundSource]InboundSourceDefinition
	retry          RetryPolicy
	limits         Limits
	retention      RetentionPolicy
}

func Compile(definition Definition) (*Catalog, error) {
	if definition.Version != DefinitionVersion {
		return nil, invalid(ErrorInvalidDefinition, "version", fmt.Sprintf("must equal %d", DefinitionVersion))
	}
	definition.Consumer = strings.TrimSpace(definition.Consumer)
	if !stableKey.MatchString(definition.Consumer) {
		return nil, invalid(ErrorInvalidDefinition, "consumer", "must be a stable lowercase key")
	}
	parsedSource, err := url.Parse(strings.TrimSpace(definition.Source))
	if err != nil || !parsedSource.IsAbs() || parsedSource.Fragment != "" {
		return nil, invalid(ErrorInvalidDefinition, "source", "must be an absolute URI without fragment")
	}
	retry, err := normalizeRetry(definition.Retry)
	if err != nil {
		return nil, err
	}
	limits, err := normalizeLimits(definition.Limits)
	if err != nil {
		return nil, err
	}
	retention, err := normalizeRetention(definition.Retention)
	if err != nil {
		return nil, err
	}
	if len(definition.EventTypes) == 0 {
		return nil, invalid(ErrorInvalidDefinition, "event_types", "must not be empty")
	}
	eventTypes := make(map[EventType]EventTypeDefinition, len(definition.EventTypes))
	for index, item := range definition.EventTypes {
		item.Type = EventType(strings.TrimSpace(string(item.Type)))
		if !validEventType(item.Type) {
			return nil, invalid(ErrorInvalidDefinition, "event_types", fmt.Sprintf("item %d has invalid type", index))
		}
		if item.MaxDataBytes == 0 {
			item.MaxDataBytes = min(64<<10, limits.MaxEventBytes)
		}
		if item.MaxDataBytes < 2 || item.MaxDataBytes > limits.MaxEventBytes {
			return nil, invalid(ErrorInvalidDefinition, "event_types", fmt.Sprintf("item %d has invalid max data bytes", index))
		}
		if item.SchemaURI != "" {
			uri, parseErr := url.Parse(item.SchemaURI)
			if parseErr != nil || !uri.IsAbs() {
				return nil, invalid(ErrorInvalidDefinition, "event_types", fmt.Sprintf("item %d has invalid schema URI", index))
			}
		}
		if _, exists := eventTypes[item.Type]; exists {
			return nil, invalid(ErrorInvalidDefinition, "event_types", fmt.Sprintf("duplicate %q", item.Type))
		}
		eventTypes[item.Type] = item
		definition.EventTypes[index] = item
	}
	inbound := make(map[InboundSource]InboundSourceDefinition, len(definition.InboundSources))
	for index, item := range definition.InboundSources {
		item.Key = InboundSource(strings.TrimSpace(string(item.Key)))
		if !stableKey.MatchString(string(item.Key)) || !stableKey.MatchString(string(item.Secret)) {
			return nil, invalid(ErrorInvalidDefinition, "inbound_sources", fmt.Sprintf("item %d has invalid key or secret ref", index))
		}
		if item.TimestampWindow == 0 {
			item.TimestampWindow = 5 * time.Minute
		}
		if item.TimestampWindow < time.Second || item.TimestampWindow > 15*time.Minute {
			return nil, invalid(ErrorInvalidDefinition, "inbound_sources", fmt.Sprintf("item %d timestamp window is invalid", index))
		}
		if item.MaxBodyBytes == 0 {
			item.MaxBodyBytes = limits.MaxEventBytes
		}
		if item.MaxBodyBytes < 2 || item.MaxBodyBytes > limits.MaxEventBytes {
			return nil, invalid(ErrorInvalidDefinition, "inbound_sources", fmt.Sprintf("item %d max body is invalid", index))
		}
		sourceURI, sourceErr := url.Parse(strings.TrimSpace(item.ExpectedSource))
		if sourceErr != nil || !sourceURI.IsAbs() || sourceURI.Fragment != "" {
			return nil, invalid(ErrorInvalidDefinition, "inbound_sources", fmt.Sprintf("item %d expected source is invalid", index))
		}
		item.ExpectedSource = sourceURI.String()
		seen := map[EventType]struct{}{}
		for _, eventType := range item.AllowedTypes {
			if _, exists := eventTypes[eventType]; !exists {
				return nil, invalid(ErrorInvalidDefinition, "inbound_sources", fmt.Sprintf("item %d references unknown type %q", index, eventType))
			}
			if _, exists := seen[eventType]; exists {
				return nil, invalid(ErrorInvalidDefinition, "inbound_sources", fmt.Sprintf("item %d repeats type %q", index, eventType))
			}
			seen[eventType] = struct{}{}
		}
		if len(seen) == 0 {
			return nil, invalid(ErrorInvalidDefinition, "inbound_sources", fmt.Sprintf("item %d must allow at least one type", index))
		}
		slices.Sort(item.AllowedTypes)
		if _, exists := inbound[item.Key]; exists {
			return nil, invalid(ErrorInvalidDefinition, "inbound_sources", fmt.Sprintf("duplicate %q", item.Key))
		}
		inbound[item.Key] = item
		definition.InboundSources[index] = item
	}
	slices.SortFunc(definition.EventTypes, func(a, b EventTypeDefinition) int {
		return strings.Compare(string(a.Type), string(b.Type))
	})
	slices.SortFunc(definition.InboundSources, func(a, b InboundSourceDefinition) int {
		return strings.Compare(string(a.Key), string(b.Key))
	})
	definition.Retry, definition.Limits, definition.Retention = retry, limits, retention
	encoded, err := json.Marshal(definition)
	if err != nil {
		return nil, unavailable("encode definition", err)
	}
	sum := sha256.Sum256(encoded)
	return &Catalog{
		version: definition.Version, consumer: definition.Consumer, source: parsedSource.String(),
		digest: hex.EncodeToString(sum[:]), eventTypes: eventTypes, inboundSources: inbound,
		retry: retry, limits: limits, retention: retention,
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
	if value.MaxAttempts == 0 {
		value.MaxAttempts = 10
	}
	if value.MaxAge == 0 {
		value.MaxAge = 72 * time.Hour
	}
	if value.BaseDelay == 0 {
		value.BaseDelay = 5 * time.Second
	}
	if value.MaxDelay == 0 {
		value.MaxDelay = 24 * time.Hour
	}
	if value.RequestTimeout == 0 {
		value.RequestTimeout = 20 * time.Second
	}
	if value.MaxRetryAfter == 0 {
		value.MaxRetryAfter = 24 * time.Hour
	}
	if value.MaxAttempts < 1 || value.MaxAttempts > 100 ||
		value.MaxAge < time.Minute || value.MaxAge > 30*24*time.Hour ||
		value.BaseDelay < time.Second || value.MaxDelay < value.BaseDelay ||
		value.RequestTimeout < time.Second || value.RequestTimeout > 30*time.Second ||
		value.MaxRetryAfter < time.Second || value.MaxRetryAfter > 7*24*time.Hour {
		return RetryPolicy{}, invalid(ErrorInvalidDefinition, "retry", "contains an out-of-range value")
	}
	return value, nil
}

func normalizeLimits(value Limits) (Limits, error) {
	if value.MaxEventBytes == 0 {
		value.MaxEventBytes = 1 << 20
	}
	if value.MaxEndpoints == 0 {
		value.MaxEndpoints = 1000
	}
	if value.MaxSubscriptions == 0 {
		value.MaxSubscriptions = 5000
	}
	if value.MaxEventTypesPerSub == 0 {
		value.MaxEventTypesPerSub = 256
	}
	if value.MaxFanout == 0 {
		value.MaxFanout = 1000
	}
	if value.MaxResponseBytes == 0 {
		value.MaxResponseBytes = 64 << 10
	}
	if value.MaxDescriptionBytes == 0 {
		value.MaxDescriptionBytes = 2000
	}
	if value.MaxIdempotencyKeyBytes == 0 {
		value.MaxIdempotencyKeyBytes = 200
	}
	if value.MaxEventBytes < 2 || value.MaxEventBytes > 1<<20 ||
		value.MaxEndpoints < 1 || value.MaxEndpoints > 10000 ||
		value.MaxSubscriptions < 1 || value.MaxSubscriptions > 100000 ||
		value.MaxEventTypesPerSub < 1 || value.MaxEventTypesPerSub > 1024 ||
		value.MaxFanout < 1 || value.MaxFanout > 10000 ||
		value.MaxResponseBytes < 1 || value.MaxResponseBytes > 1<<20 ||
		value.MaxDescriptionBytes < 1 || value.MaxDescriptionBytes > 64<<10 ||
		value.MaxIdempotencyKeyBytes < 16 || value.MaxIdempotencyKeyBytes > 1024 {
		return Limits{}, invalid(ErrorInvalidDefinition, "limits", "contains an out-of-range value")
	}
	return value, nil
}

func normalizeRetention(value RetentionPolicy) (RetentionPolicy, error) {
	if value.Events == 0 {
		value.Events = 90 * 24 * time.Hour
	}
	if value.Attempts == 0 {
		value.Attempts = 90 * 24 * time.Hour
	}
	if value.InboundReceipts == 0 {
		value.InboundReceipts = 90 * 24 * time.Hour
	}
	for _, duration := range []time.Duration{value.Events, value.Attempts, value.InboundReceipts} {
		if duration < 24*time.Hour || duration > 10*365*24*time.Hour {
			return RetentionPolicy{}, invalid(ErrorInvalidDefinition, "retention", "contains an out-of-range value")
		}
	}
	return value, nil
}

func validEventType(value EventType) bool {
	text := string(value)
	return len(text) >= 5 && len(text) <= 255 && strings.Contains(text, ".") &&
		!strings.ContainsAny(text, " \t\r\n\x00")
}

func (catalog *Catalog) Version() uint64 {
	if catalog == nil {
		return 0
	}
	return catalog.version
}
func (catalog *Catalog) Consumer() string {
	if catalog == nil {
		return ""
	}
	return catalog.consumer
}
func (catalog *Catalog) Source() string {
	if catalog == nil {
		return ""
	}
	return catalog.source
}
func (catalog *Catalog) Digest() string {
	if catalog == nil {
		return ""
	}
	return catalog.digest
}
func (catalog *Catalog) Retry() RetryPolicy {
	if catalog == nil {
		return RetryPolicy{}
	}
	return catalog.retry
}
func (catalog *Catalog) Limits() Limits {
	if catalog == nil {
		return Limits{}
	}
	return catalog.limits
}
func (catalog *Catalog) Retention() RetentionPolicy {
	if catalog == nil {
		return RetentionPolicy{}
	}
	return catalog.retention
}
