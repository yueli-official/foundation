package webhook

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MemoryOptions struct {
	Clock      func() time.Time
	Scheduler  Scheduler
	Secrets    SecretStore
	Authorizer NetworkAuthorizer
}

type memoryEvent struct {
	view        EventView
	fingerprint string
}

type memoryEndpoint struct {
	current   Endpoint
	secret    SecretRef
	revisions map[uint64]Endpoint
}

type memoryInbound struct {
	receipt InboundReceipt
}

type Memory struct {
	mu            sync.RWMutex
	catalog       *Catalog
	clock         func() time.Time
	scheduler     Scheduler
	secrets       SecretStore
	authorizer    NetworkAuthorizer
	events        map[EventID]memoryEvent
	eventKeys     map[string]EventID
	endpoints     map[EndpointID]memoryEndpoint
	subscriptions map[SubscriptionID]Subscription
	deliveries    map[DeliveryID]DeliveryView
	attempts      map[DeliveryID][]AttemptView
	inbound       map[string]memoryInbound
	replays       map[string]DeliveryID
}

var _ Runtime = (*Memory)(nil)

func NewMemory(catalog *Catalog, options MemoryOptions) (*Memory, error) {
	if catalog == nil {
		return nil, invalid(ErrorInvalidDefinition, "catalog", "is required")
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.Secrets == nil {
		options.Secrets = NewMemorySecretStore()
	}
	return &Memory{
		catalog: catalog, clock: options.Clock, scheduler: options.Scheduler, secrets: options.Secrets,
		authorizer: options.Authorizer,
		events:     map[EventID]memoryEvent{}, eventKeys: map[string]EventID{},
		endpoints: map[EndpointID]memoryEndpoint{}, subscriptions: map[SubscriptionID]Subscription{},
		deliveries: map[DeliveryID]DeliveryView{}, attempts: map[DeliveryID][]AttemptView{},
		inbound: map[string]memoryInbound{}, replays: map[string]DeliveryID{},
	}, nil
}

func (memory *Memory) PutEndpoint(ctx context.Context, command PutEndpointCommand) (EndpointCredential, error) {
	if err := ctx.Err(); err != nil {
		return EndpointCredential{}, err
	}
	route, err := memory.authorizer.Authorize(ctx, command.URL)
	if err != nil {
		return EndpointCredential{}, err
	}
	canonical := route.URL.String()
	if len(command.Description) > memory.catalog.limits.MaxDescriptionBytes {
		return EndpointCredential{}, invalid(ErrorLimitExceeded, "description", "is too large")
	}
	now := memory.clock().UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	id := command.ID
	if id == "" {
		if len(memory.endpoints) >= memory.catalog.limits.MaxEndpoints {
			return EndpointCredential{}, invalid(ErrorLimitExceeded, "endpoints", "limit reached")
		}
		generated, generateErr := NewID()
		if generateErr != nil {
			return EndpointCredential{}, generateErr
		}
		id = EndpointID(generated)
	} else if !stableKey.MatchString(string(id)) {
		return EndpointCredential{}, invalid(ErrorInvalidEvent, "endpoint_id", "is invalid")
	}
	existing, exists := memory.endpoints[id]
	if exists && command.ExpectedETag != existing.current.ETag {
		return EndpointCredential{}, &Error{Code: ErrorETagConflict, Field: "expected_etag", Message: "does not match"}
	}
	revision := uint64(1)
	createdAt := now
	state := EndpointActive
	secretRef := SecretRef("webhook.endpoint." + string(id))
	var plaintext string
	if exists {
		revision = existing.current.Revision + 1
		createdAt, state, secretRef = existing.current.CreatedAt, existing.current.State, existing.secret
	} else {
		secret, secretErr := newSecret()
		if secretErr != nil {
			return EndpointCredential{}, secretErr
		}
		plaintext = secret
		decoded, _ := decodeSecret(secret)
		material := SecretMaterial{
			Revision: SecretRevision(fmt.Sprintf("r%d", revision)),
			Value:    decoded, NotBefore: now,
		}
		if createErr := memory.secrets.Create(ctx, secretRef, material); createErr != nil {
			return EndpointCredential{}, createErr
		}
	}
	endpoint := Endpoint{
		ID: id, Revision: revision, URL: canonical, Description: strings.TrimSpace(command.Description),
		State: state, CreatedAt: createdAt, UpdatedAt: now,
	}
	endpoint.ETag = endpointETag(endpoint)
	revisions := map[uint64]Endpoint{}
	if exists {
		revisions = existing.revisions
	}
	revisions[revision] = endpoint
	memory.endpoints[id] = memoryEndpoint{current: endpoint, secret: secretRef, revisions: revisions}
	return EndpointCredential{Endpoint: endpoint, Secret: plaintext}, nil
}

func (memory *Memory) PutSubscription(ctx context.Context, command PutSubscriptionCommand) (Subscription, error) {
	if err := ctx.Err(); err != nil {
		return Subscription{}, err
	}
	if len(command.EventTypes) == 0 || len(command.EventTypes) > memory.catalog.limits.MaxEventTypesPerSub {
		return Subscription{}, invalid(ErrorLimitExceeded, "event_types", "count is invalid")
	}
	types := append([]EventType(nil), command.EventTypes...)
	slices.Sort(types)
	types = slices.Compact(types)
	for _, eventType := range types {
		if _, exists := memory.catalog.eventTypes[eventType]; !exists {
			return Subscription{}, invalid(ErrorInvalidEvent, "event_types", fmt.Sprintf("%q is not registered", eventType))
		}
	}
	now := memory.clock().UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if _, exists := memory.endpoints[command.EndpointID]; !exists {
		return Subscription{}, &Error{Code: ErrorNotFound, Field: "endpoint_id", Message: "does not exist"}
	}
	id := command.ID
	if id == "" {
		if len(memory.subscriptions) >= memory.catalog.limits.MaxSubscriptions {
			return Subscription{}, invalid(ErrorLimitExceeded, "subscriptions", "limit reached")
		}
		generated, err := NewID()
		if err != nil {
			return Subscription{}, err
		}
		id = SubscriptionID(generated)
	} else if !stableKey.MatchString(string(id)) {
		return Subscription{}, invalid(ErrorInvalidEvent, "subscription_id", "is invalid")
	}
	existing, exists := memory.subscriptions[id]
	if exists && command.ExpectedETag != existing.ETag {
		return Subscription{}, &Error{Code: ErrorETagConflict, Field: "expected_etag", Message: "does not match"}
	}
	revision, createdAt := uint64(1), now
	if exists {
		revision, createdAt = existing.Revision+1, existing.CreatedAt
	}
	subscription := Subscription{
		ID: id, Revision: revision, EndpointID: command.EndpointID, EventTypes: types,
		Enabled: command.Enabled, Description: strings.TrimSpace(command.Description),
		CreatedAt: createdAt, UpdatedAt: now,
	}
	subscription.ETag = subscriptionETag(subscription)
	memory.subscriptions[id] = subscription
	return subscription, nil
}

func (memory *Memory) SetEndpointState(ctx context.Context, command SetEndpointStateCommand) (Endpoint, error) {
	if err := ctx.Err(); err != nil {
		return Endpoint{}, err
	}
	if command.State != EndpointActive && command.State != EndpointPaused &&
		command.State != EndpointDisabled && command.State != EndpointRevoked {
		return Endpoint{}, invalid(ErrorStateConflict, "state", "is invalid")
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	record, exists := memory.endpoints[command.EndpointID]
	if !exists {
		return Endpoint{}, &Error{Code: ErrorNotFound, Field: "endpoint_id", Message: "does not exist"}
	}
	if command.ExpectedETag != record.current.ETag {
		return Endpoint{}, &Error{Code: ErrorETagConflict, Field: "expected_etag", Message: "does not match"}
	}
	if record.current.State == EndpointRevoked {
		return Endpoint{}, &Error{Code: ErrorStateConflict, Field: "state", Message: "revoked endpoint cannot transition"}
	}
	record.current.Revision++
	record.current.State = command.State
	record.current.UpdatedAt = memory.clock().UTC()
	record.current.ETag = endpointETag(record.current)
	record.revisions[record.current.Revision] = record.current
	if command.State == EndpointActive {
		for id, delivery := range memory.deliveries {
			if delivery.EndpointID != command.EndpointID || delivery.State != DeliveryPaused {
				continue
			}
			delivery.State = DeliveryPending
			delivery.NextAttemptAt = record.current.UpdatedAt
			delivery.UpdatedAt = record.current.UpdatedAt
			if memory.scheduler != nil {
				if err := memory.scheduler.Enqueue(ctx, DeliveryWork{
					DeliveryID: id,
					RunAt:      record.current.UpdatedAt,
					Key:        fmt.Sprintf("webhook.delivery:%s:resume:%d", id, record.current.Revision),
				}); err != nil {
					return Endpoint{}, err
				}
			}
			memory.deliveries[id] = delivery
		}
	}
	memory.endpoints[command.EndpointID] = record
	return record.current, nil
}

func (memory *Memory) RotateSecret(ctx context.Context, command RotateSecretCommand) (EndpointCredential, error) {
	if err := ctx.Err(); err != nil {
		return EndpointCredential{}, err
	}
	if command.Overlap <= 0 || command.Overlap > 7*24*time.Hour {
		return EndpointCredential{}, invalid(ErrorInvalidEvent, "overlap", "must be positive and at most seven days")
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	record, exists := memory.endpoints[command.EndpointID]
	if !exists {
		return EndpointCredential{}, &Error{Code: ErrorNotFound, Field: "endpoint_id", Message: "does not exist"}
	}
	if record.current.State == EndpointRevoked {
		return EndpointCredential{}, &Error{Code: ErrorStateConflict, Field: "state", Message: "endpoint is revoked"}
	}
	secret, err := newSecret()
	if err != nil {
		return EndpointCredential{}, err
	}
	value, _ := decodeSecret(secret)
	now := memory.clock().UTC()
	material := SecretMaterial{
		Revision: SecretRevision(fmt.Sprintf("r%d", record.current.Revision+1)),
		Value:    value, NotBefore: now,
	}
	if err := memory.secrets.Rotate(ctx, record.secret, material, now.Add(command.Overlap)); err != nil {
		return EndpointCredential{}, err
	}
	record.current.Revision++
	record.current.UpdatedAt = now
	record.current.ETag = endpointETag(record.current)
	record.revisions[record.current.Revision] = record.current
	memory.endpoints[command.EndpointID] = record
	return EndpointCredential{Endpoint: record.current, Secret: secret}, nil
}

func (memory *Memory) Publish(ctx context.Context, command EventCommand) (EventReceipt, error) {
	if err := ctx.Err(); err != nil {
		return EventReceipt{}, err
	}
	now := memory.clock().UTC()
	prepared, err := memory.prepareEvent(now, command)
	if err != nil {
		return EventReceipt{}, err
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if existingID, exists := memory.eventKeys[command.IdempotencyKey]; exists {
		existing := memory.events[existingID]
		if existing.fingerprint != prepared.fingerprint {
			return EventReceipt{}, &Error{Code: ErrorIdempotency, Field: "idempotency_key", Message: "was used for different event"}
		}
		count := 0
		for _, delivery := range memory.deliveries {
			if delivery.EventID == existingID {
				count++
			}
		}
		return EventReceipt{
			EventID: existingID, Deliveries: count, Duplicate: true,
			BodyDigest: existing.view.BodyDigest, PublishedAt: existing.view.PublishedAt,
		}, nil
	}
	eventIDText, err := NewID()
	if err != nil {
		return EventReceipt{}, err
	}
	eventID := EventID(eventIDText)
	body, err := encodeCloudEvent(memory.catalog, eventID, command)
	if err != nil {
		return EventReceipt{}, unavailable("encode CloudEvent", err)
	}
	if len(body) > memory.catalog.limits.MaxEventBytes {
		return EventReceipt{}, invalid(ErrorEventTooLarge, "event", "encoded CloudEvent exceeds limit")
	}
	view := EventView{
		ID: eventID, Type: command.Type, Subject: command.Subject, Body: body,
		BodyDigest: digestBytes(body), IdempotencyKey: command.IdempotencyKey,
		OccurredAt: command.OccurredAt.UTC(), PublishedAt: now,
	}
	var scheduled []DeliveryWork
	for _, subscription := range memory.subscriptions {
		endpoint := memory.endpoints[subscription.EndpointID]
		if !subscription.Enabled || endpoint.current.State != EndpointActive ||
			!slices.Contains(subscription.EventTypes, command.Type) {
			continue
		}
		if len(scheduled) >= memory.catalog.limits.MaxFanout {
			return EventReceipt{}, invalid(ErrorLimitExceeded, "fanout", "exceeds configured maximum")
		}
		idText, idErr := NewID()
		if idErr != nil {
			return EventReceipt{}, idErr
		}
		id := DeliveryID(idText)
		delivery := DeliveryView{
			ID: id, EventID: eventID, EndpointID: endpoint.current.ID,
			EndpointRevision: endpoint.current.Revision, SubscriptionID: subscription.ID,
			SubscriptionRevision: subscription.Revision, State: DeliveryPending,
			NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
		}
		memory.deliveries[id] = delivery
		scheduled = append(scheduled, DeliveryWork{
			DeliveryID: id, RunAt: now, Key: "webhook.delivery:" + string(id),
		})
	}
	if memory.scheduler != nil {
		for _, item := range scheduled {
			if err := memory.scheduler.Enqueue(ctx, item); err != nil {
				for _, rollback := range scheduled {
					delete(memory.deliveries, rollback.DeliveryID)
				}
				return EventReceipt{}, err
			}
		}
	}
	memory.events[eventID] = memoryEvent{view: view, fingerprint: prepared.fingerprint}
	memory.eventKeys[command.IdempotencyKey] = eventID
	return EventReceipt{
		EventID: eventID, Deliveries: len(scheduled), BodyDigest: view.BodyDigest, PublishedAt: now,
	}, nil
}

type preparedEvent struct{ fingerprint string }

func (memory *Memory) prepareEvent(now time.Time, command EventCommand) (preparedEvent, error) {
	return prepareEvent(memory.catalog, now, command)
}

func prepareEvent(catalog *Catalog, now time.Time, command EventCommand) (preparedEvent, error) {
	definition, exists := catalog.eventTypes[command.Type]
	if !exists {
		return preparedEvent{}, invalid(ErrorInvalidEvent, "type", "is not registered")
	}
	command.Subject = strings.TrimSpace(command.Subject)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.IdempotencyKey == "" || len(command.IdempotencyKey) > catalog.limits.MaxIdempotencyKeyBytes {
		return preparedEvent{}, invalid(ErrorInvalidEvent, "idempotency_key", "is required and must fit the configured limit")
	}
	if command.OccurredAt.IsZero() || command.OccurredAt.After(now.Add(5*time.Minute)) {
		return preparedEvent{}, invalid(ErrorInvalidEvent, "occurred_at", "is missing or too far in the future")
	}
	if len(command.Data) > definition.MaxDataBytes || !json.Valid(command.Data) {
		return preparedEvent{}, invalid(ErrorInvalidEvent, "data", "must be valid bounded JSON")
	}
	canonical := struct {
		Type        EventType
		Subject     string
		Data        json.RawMessage
		OccurredAt  time.Time
		TraceParent string
	}{command.Type, command.Subject, command.Data, command.OccurredAt.UTC(), command.TraceParent}
	encoded, _ := json.Marshal(canonical)
	return preparedEvent{fingerprint: digestBytes(encoded)}, nil
}

func (memory *Memory) Event(ctx context.Context, id EventID) (EventView, error) {
	if err := ctx.Err(); err != nil {
		return EventView{}, err
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	event, exists := memory.events[id]
	if !exists {
		return EventView{}, &Error{Code: ErrorNotFound, Field: "event_id", Message: "does not exist"}
	}
	event.view.Body = append([]byte(nil), event.view.Body...)
	return event.view, nil
}

func (memory *Memory) Delivery(ctx context.Context, id DeliveryID) (DeliveryView, error) {
	if err := ctx.Err(); err != nil {
		return DeliveryView{}, err
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	value, exists := memory.deliveries[id]
	if !exists {
		return DeliveryView{}, &Error{Code: ErrorNotFound, Field: "delivery_id", Message: "does not exist"}
	}
	return value, nil
}

func (memory *Memory) ListDeliveries(ctx context.Context, query DeliveryQuery) (DeliveryPage, error) {
	if err := ctx.Err(); err != nil {
		return DeliveryPage{}, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 500 {
		return DeliveryPage{}, invalid(ErrorLimitExceeded, "limit", "must be between 1 and 500")
	}
	memory.mu.RLock()
	result := make([]DeliveryView, 0, min(limit+1, len(memory.deliveries)))
	for _, delivery := range memory.deliveries {
		event := memory.events[delivery.EventID].view
		if query.EndpointID != "" && delivery.EndpointID != query.EndpointID {
			continue
		}
		if query.EventType != "" && event.Type != query.EventType {
			continue
		}
		if len(query.States) > 0 && !slices.Contains(query.States, delivery.State) {
			continue
		}
		if !query.Since.IsZero() && delivery.CreatedAt.Before(query.Since) {
			continue
		}
		if !query.Until.IsZero() && delivery.CreatedAt.After(query.Until) {
			continue
		}
		if query.After != "" && string(delivery.ID) <= query.After {
			continue
		}
		result = append(result, delivery)
	}
	memory.mu.RUnlock()
	slices.SortFunc(result, func(a, b DeliveryView) int { return strings.Compare(string(a.ID), string(b.ID)) })
	page := DeliveryPage{}
	if len(result) > limit {
		page.Next, result = string(result[limit-1].ID), result[:limit]
	}
	page.Deliveries = result
	return page, nil
}

func (memory *Memory) Attempts(ctx context.Context, id DeliveryID) ([]AttemptView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	if _, exists := memory.deliveries[id]; !exists {
		return nil, &Error{Code: ErrorNotFound, Field: "delivery_id", Message: "does not exist"}
	}
	return append([]AttemptView(nil), memory.attempts[id]...), nil
}

func (memory *Memory) Replay(ctx context.Context, command ReplayCommand) (ReplayReceipt, error) {
	if err := ctx.Err(); err != nil {
		return ReplayReceipt{}, err
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" || strings.TrimSpace(command.Reason) == "" {
		return ReplayReceipt{}, invalid(ErrorInvalidEvent, "replay", "reason and idempotency key are required")
	}
	now := memory.clock().UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if existingID, exists := memory.replays[command.IdempotencyKey]; exists {
		return ReplayReceipt{Delivery: memory.deliveries[existingID], Duplicate: true}, nil
	}
	original, exists := memory.deliveries[command.DeliveryID]
	if !exists {
		return ReplayReceipt{}, &Error{Code: ErrorNotFound, Field: "delivery_id", Message: "does not exist"}
	}
	if original.State != DeliveryFailed && original.State != DeliveryCancelled {
		return ReplayReceipt{}, &Error{Code: ErrorStateConflict, Field: "delivery", Message: "is not replayable"}
	}
	endpoint := memory.endpoints[original.EndpointID]
	if endpoint.current.State != EndpointActive {
		return ReplayReceipt{}, &Error{Code: ErrorStateConflict, Field: "endpoint", Message: "is not active"}
	}
	idText, err := NewID()
	if err != nil {
		return ReplayReceipt{}, err
	}
	id := DeliveryID(idText)
	replay := DeliveryView{
		ID: id, EventID: original.EventID, EndpointID: endpoint.current.ID,
		EndpointRevision: endpoint.current.Revision, SubscriptionID: original.SubscriptionID,
		SubscriptionRevision: original.SubscriptionRevision, State: DeliveryPending,
		NextAttemptAt: now, ReplayOf: original.ID, CreatedAt: now, UpdatedAt: now,
	}
	if memory.scheduler != nil {
		if err := memory.scheduler.Enqueue(ctx, DeliveryWork{
			DeliveryID: id, RunAt: now, Key: "webhook.delivery:" + string(id),
		}); err != nil {
			return ReplayReceipt{}, err
		}
	}
	memory.deliveries[id] = replay
	memory.replays[command.IdempotencyKey] = id
	return ReplayReceipt{Delivery: replay}, nil
}

func (memory *Memory) Snapshot(ctx context.Context) (MetricsSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return MetricsSnapshot{}, err
	}
	now := memory.clock().UTC()
	result := MetricsSnapshot{ByState: map[DeliveryState]int64{}}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	for _, delivery := range memory.deliveries {
		result.ByState[delivery.State]++
		if (delivery.State == DeliveryPending || delivery.State == DeliveryRetrying) && !delivery.NextAttemptAt.After(now) {
			result.Due++
			if result.OldestDueAt == nil || delivery.NextAttemptAt.Before(*result.OldestDueAt) {
				value := delivery.NextAttemptAt
				result.OldestDueAt = &value
			}
		}
	}
	return result, nil
}

func (memory *Memory) Verify(ctx context.Context, source InboundSource, message IncomingMessage) (VerifiedInbound, error) {
	return verifyInbound(ctx, memory.catalog, memory.secrets, memory.clock, source, message)
}

func verifyInbound(
	ctx context.Context,
	catalog *Catalog,
	secretStore SecretStore,
	clock func() time.Time,
	source InboundSource,
	message IncomingMessage,
) (VerifiedInbound, error) {
	if err := ctx.Err(); err != nil {
		return VerifiedInbound{}, err
	}
	definition, exists := catalog.inboundSources[source]
	if !exists {
		return VerifiedInbound{}, &Error{Code: ErrorNotFound, Field: "source", Message: "is not registered"}
	}
	if len(message.Body) > definition.MaxBodyBytes {
		return VerifiedInbound{}, invalid(ErrorLimitExceeded, "body", "exceeds configured limit")
	}
	receivedAt := message.ReceivedAt.UTC()
	if receivedAt.IsZero() {
		receivedAt = clock().UTC()
	}
	id := singleHeader(message.Headers, HeaderWebhookID)
	timestampText := singleHeader(message.Headers, HeaderWebhookTimestamp)
	signatures := message.Headers.Values(HeaderWebhookSignature)
	if id == "" || timestampText == "" || len(signatures) == 0 {
		return VerifiedInbound{}, invalid(ErrorSignatureMissing, "headers", "required headers are missing")
	}
	seconds, err := strconv.ParseInt(timestampText, 10, 64)
	if err != nil {
		return VerifiedInbound{}, invalid(ErrorSignatureInvalid, "webhook_timestamp", "is invalid")
	}
	attemptedAt := time.Unix(seconds, 0).UTC()
	if attemptedAt.Before(receivedAt.Add(-definition.TimestampWindow)) || attemptedAt.After(receivedAt.Add(definition.TimestampWindow)) {
		return VerifiedInbound{}, invalid(ErrorTimestampWindow, "webhook_timestamp", "is outside the permitted window")
	}
	secrets, err := secretStore.Resolve(ctx, definition.Secret, receivedAt)
	if err != nil {
		return VerifiedInbound{}, err
	}
	candidates := append([]SecretMaterial{secrets.Primary}, secrets.Previous...)
	revision, err := VerifyV1(id, timestampText, message.Body, signatures, candidates)
	if err != nil {
		return VerifiedInbound{}, err
	}
	var envelope cloudEvent
	if err := json.Unmarshal(message.Body, &envelope); err != nil {
		return VerifiedInbound{}, invalid(ErrorEnvelopeInvalid, "body", "is not a CloudEvent")
	}
	if envelope.SpecVersion != "1.0" || string(envelope.ID) != id || envelope.Source != definition.ExpectedSource {
		return VerifiedInbound{}, invalid(ErrorEnvelopeInvalid, "body", "identity fields do not match")
	}
	if !slices.Contains(definition.AllowedTypes, envelope.Type) {
		return VerifiedInbound{}, invalid(ErrorTypeForbidden, "type", "is not permitted for source")
	}
	return VerifiedInbound{
		source: source, eventID: id, eventType: envelope.Type, occurredAt: envelope.Time.UTC(),
		attemptedAt: attemptedAt, receivedAt: receivedAt, body: append([]byte(nil), message.Body...),
		bodyDigest: digestBytes(message.Body), secretRevision: revision, catalogDigest: catalog.digest,
	}, nil
}

func (memory *Memory) Accept(ctx context.Context, verified VerifiedInbound) (InboundReceipt, error) {
	if err := ctx.Err(); err != nil {
		return InboundReceipt{}, err
	}
	if verified.catalogDigest != memory.catalog.digest || verified.source == "" || verified.eventID == "" {
		return InboundReceipt{}, invalid(ErrorSignatureInvalid, "verified", "was not produced by this runtime")
	}
	key := string(verified.source) + "\x00" + verified.eventID
	now := memory.clock().UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if existing, exists := memory.inbound[key]; exists {
		if existing.receipt.BodyDigest != verified.bodyDigest {
			return InboundReceipt{}, &Error{Code: ErrorIdempotency, Field: "event_id", Message: "was previously accepted with different content"}
		}
		result := existing.receipt
		result.FirstSeen = false
		return result, nil
	}
	id, err := NewID()
	if err != nil {
		return InboundReceipt{}, err
	}
	receipt := InboundReceipt{
		ReceiptID: id, Source: verified.source, EventID: verified.eventID,
		BodyDigest: verified.bodyDigest, KeyRevision: verified.secretRevision,
		FirstSeen: true, ReceivedAt: verified.receivedAt, AcceptedAt: now,
	}
	memory.inbound[key] = memoryInbound{receipt: receipt}
	return receipt, nil
}

func (memory *Memory) BeginAttempt(ctx context.Context, id DeliveryID) (AttemptPlan, error) {
	if err := ctx.Err(); err != nil {
		return AttemptPlan{}, err
	}
	now := memory.clock().UTC()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	delivery, exists := memory.deliveries[id]
	if !exists {
		return AttemptPlan{}, &Error{Code: ErrorNotFound, Field: "delivery_id", Message: "does not exist"}
	}
	if delivery.State == DeliveryDelivered || delivery.State == DeliveryFailed || delivery.State == DeliveryCancelled {
		return AttemptPlan{}, &Error{Code: ErrorStateConflict, Field: "delivery", Message: "is terminal"}
	}
	if delivery.State == DeliveryDelivering {
		attempts := memory.attempts[id]
		if len(attempts) > 0 && attempts[len(attempts)-1].FinishedAt == nil {
			lease := max(2*memory.catalog.retry.RequestTimeout, time.Minute)
			if now.Before(attempts[len(attempts)-1].StartedAt.Add(lease)) {
				return AttemptPlan{}, &Error{Code: ErrorUnavailable, Field: "attempt", Message: "another attempt is still live", Retryable: true}
			}
			attempts[len(attempts)-1].Outcome = AttemptUnknown
			attempts[len(attempts)-1].ErrorCode = "lease_expired"
			finished := now
			attempts[len(attempts)-1].FinishedAt = &finished
			memory.attempts[id] = attempts
		}
	}
	endpoint := memory.endpoints[delivery.EndpointID]
	if endpoint.current.State == EndpointPaused {
		delivery.State, delivery.UpdatedAt = DeliveryPaused, now
		memory.deliveries[id] = delivery
		return AttemptPlan{}, &Error{Code: ErrorUnavailable, Field: "endpoint", Message: "is paused", Retryable: true}
	}
	if endpoint.current.State == EndpointDisabled || endpoint.current.State == EndpointRevoked {
		delivery.State, delivery.LastErrorCode, delivery.UpdatedAt = DeliveryCancelled, "endpoint_disabled", now
		memory.deliveries[id] = delivery
		return AttemptPlan{}, &Error{Code: ErrorStateConflict, Field: "endpoint", Message: "is disabled"}
	}
	revision, exists := endpoint.revisions[delivery.EndpointRevision]
	if !exists {
		return AttemptPlan{}, &Error{Code: ErrorStateConflict, Field: "endpoint_revision", Message: "does not exist"}
	}
	event := memory.events[delivery.EventID].view
	attemptText, err := NewID()
	if err != nil {
		return AttemptPlan{}, err
	}
	delivery.AttemptCount++
	delivery.State, delivery.NextAttemptAt, delivery.UpdatedAt = DeliveryDelivering, time.Time{}, now
	memory.deliveries[id] = delivery
	attempt := AttemptView{
		ID: AttemptID(attemptText), DeliveryID: id, Number: delivery.AttemptCount,
		Outcome: AttemptUnknown, RequestDigest: event.BodyDigest, StartedAt: now,
	}
	memory.attempts[id] = append(memory.attempts[id], attempt)
	return AttemptPlan{
		AttemptID: attempt.ID, DeliveryID: id, EventID: event.ID, EventType: event.Type,
		EndpointID: endpoint.current.ID, URL: revision.URL, Secret: endpoint.secret,
		Body: append([]byte(nil), event.Body...), BodyDigest: event.BodyDigest,
		Number: delivery.AttemptCount, DeliveryCreated: delivery.CreatedAt,
	}, nil
}

func (memory *Memory) CompleteAttempt(ctx context.Context, command AttemptCompletion) (DeliveryView, error) {
	if err := ctx.Err(); err != nil {
		return DeliveryView{}, err
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	delivery, exists := memory.deliveries[command.Plan.DeliveryID]
	if !exists {
		return DeliveryView{}, &Error{Code: ErrorNotFound, Field: "delivery_id", Message: "does not exist"}
	}
	if delivery.State != DeliveryDelivering {
		return DeliveryView{}, &Error{Code: ErrorStateConflict, Field: "delivery", Message: "is no longer delivering"}
	}
	attempts := memory.attempts[delivery.ID]
	if len(attempts) == 0 || attempts[len(attempts)-1].ID != command.Plan.AttemptID ||
		attempts[len(attempts)-1].FinishedAt != nil {
		return DeliveryView{}, &Error{Code: ErrorStateConflict, Field: "attempt", Message: "is not current"}
	}
	finished := command.FinishedAt
	attempts[len(attempts)-1].Outcome = command.Outcome
	attempts[len(attempts)-1].StatusCode = command.StatusCode
	attempts[len(attempts)-1].ErrorCode = command.ErrorCode
	attempts[len(attempts)-1].ResponseDigest = command.ResponseDigest
	attempts[len(attempts)-1].SecretRevision = command.SecretRevision
	attempts[len(attempts)-1].FinishedAt = &finished
	memory.attempts[delivery.ID] = attempts
	switch command.Outcome {
	case AttemptSucceeded:
		delivery.State = DeliveryDelivered
	case AttemptRetryable:
		delivery.State, delivery.NextAttemptAt = DeliveryRetrying, command.NextAttemptAt
	case AttemptPermanent:
		delivery.State = DeliveryFailed
	default:
		return DeliveryView{}, invalid(ErrorStateConflict, "outcome", "is invalid")
	}
	delivery.LastErrorCode, delivery.UpdatedAt = command.ErrorCode, command.FinishedAt
	memory.deliveries[delivery.ID] = delivery
	if command.DisableEndpoint {
		endpoint := memory.endpoints[delivery.EndpointID]
		if endpoint.current.State != EndpointRevoked {
			endpoint.current.Revision++
			endpoint.current.State, endpoint.current.UpdatedAt = EndpointDisabled, command.FinishedAt
			endpoint.current.ETag = endpointETag(endpoint.current)
			endpoint.revisions[endpoint.current.Revision] = endpoint.current
			memory.endpoints[delivery.EndpointID] = endpoint
		}
		for otherID, other := range memory.deliveries {
			if otherID != delivery.ID && other.EndpointID == delivery.EndpointID &&
				(other.State == DeliveryPending || other.State == DeliveryRetrying || other.State == DeliveryPaused) {
				other.State, other.LastErrorCode, other.UpdatedAt = DeliveryCancelled, "endpoint_disabled", command.FinishedAt
				memory.deliveries[otherID] = other
			}
		}
	}
	return delivery, nil
}

func endpointETag(value Endpoint) string {
	encoded, _ := json.Marshal(struct {
		ID          EndpointID
		Revision    uint64
		URL         string
		Description string
		State       EndpointState
	}{value.ID, value.Revision, value.URL, value.Description, value.State})
	return digestBytes(encoded)
}

func subscriptionETag(value Subscription) string {
	encoded, _ := json.Marshal(struct {
		ID          SubscriptionID
		Revision    uint64
		EndpointID  EndpointID
		EventTypes  []EventType
		Enabled     bool
		Description string
	}{value.ID, value.Revision, value.EndpointID, value.EventTypes, value.Enabled, value.Description})
	return digestBytes(encoded)
}

func newSecret() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", unavailable("generate secret", err)
	}
	return "whsec_" + base64.StdEncoding.EncodeToString(value), nil
}

func decodeSecret(value string) ([]byte, error) {
	if !strings.HasPrefix(value, "whsec_") {
		return nil, invalid(ErrorInvalidEvent, "secret", "has invalid prefix")
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "whsec_"))
	if err != nil || len(decoded) < 24 || len(decoded) > 64 {
		return nil, invalid(ErrorInvalidEvent, "secret", "has invalid encoding")
	}
	return decoded, nil
}

func singleHeader(headers http.Header, key string) string {
	values := headers.Values(key)
	if len(values) != 1 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func ValidateEndpointURL(raw string, policy NetworkPolicy) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", invalid(ErrorEndpointUnsafe, "url", "must be absolute")
	}
	if parsed.Scheme != "https" && !(policy.AllowHTTP && parsed.Scheme == "http") {
		return "", invalid(ErrorEndpointUnsafe, "url", "scheme is not allowed")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", invalid(ErrorEndpointUnsafe, "url", "userinfo and fragments are forbidden")
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || !slices.Contains(policy.AllowedPorts, uint16(portNumber)) {
		return "", invalid(ErrorEndpointUnsafe, "url", "port is not allowed")
	}
	parsed.Host = strings.ToLower(parsed.Host)
	return parsed.String(), nil
}
