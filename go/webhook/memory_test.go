package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"
)

type captureScheduler struct {
	mu    sync.Mutex
	items []DeliveryWork
}

func (scheduler *captureScheduler) Enqueue(_ context.Context, work DeliveryWork) error {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	scheduler.items = append(scheduler.items, work)
	return nil
}

func TestMemoryPublishFanoutIdempotencyAndReplay(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	catalog := MustCompile(testDefinition())
	scheduler := &captureScheduler{}
	secrets := NewMemorySecretStore()
	runtime, err := NewMemory(catalog, MemoryOptions{
		Clock: func() time.Time { return now }, Scheduler: scheduler, Secrets: secrets,
		Authorizer: testAuthorizer(now),
	})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := runtime.PutEndpoint(context.Background(), PutEndpointCommand{
		ID: "primary", URL: "https://example.com/hooks",
	})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Secret == "" {
		t.Fatal("endpoint secret was not returned on create")
	}
	_, err = runtime.PutSubscription(context.Background(), PutSubscriptionCommand{
		ID: "all-created", EndpointID: endpoint.Endpoint.ID,
		EventTypes: []EventType{"com.yueli.test.created.v1"}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := EventCommand{
		Type: "com.yueli.test.created.v1", Subject: "thing/1",
		Data: json.RawMessage(`{"id":"1"}`), OccurredAt: now,
		IdempotencyKey: "thing:1:created:v1",
	}
	first, err := runtime.Publish(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if first.Deliveries != 1 || first.Duplicate {
		t.Fatalf("first receipt=%+v", first)
	}
	second, err := runtime.Publish(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate || second.EventID != first.EventID {
		t.Fatalf("duplicate receipt=%+v", second)
	}
	command.Data = json.RawMessage(`{"id":"different"}`)
	if _, err := runtime.Publish(context.Background(), command); !IsCode(err, ErrorIdempotency) {
		t.Fatalf("idempotency conflict err=%v", err)
	}
	event, err := runtime.Event(context.Background(), first.EventID)
	if err != nil {
		t.Fatal(err)
	}
	var envelope cloudEvent
	if err := json.Unmarshal(event.Body, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.ID != first.EventID || envelope.Source != catalog.Source() {
		t.Fatalf("CloudEvent=%+v", envelope)
	}
	deliveryID := scheduler.items[0].DeliveryID
	memoryDelivery := runtime.deliveries[deliveryID]
	memoryDelivery.State = DeliveryFailed
	runtime.deliveries[deliveryID] = memoryDelivery
	replay, err := runtime.Replay(context.Background(), ReplayCommand{
		DeliveryID: deliveryID, Reason: "operator_retry", IdempotencyKey: "replay-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if replay.Delivery.ID == deliveryID || replay.Delivery.EventID != first.EventID || replay.Delivery.ReplayOf != deliveryID {
		t.Fatalf("replay=%+v", replay)
	}
}

func TestInboundVerifyAndReceiptConflict(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	catalog := MustCompile(testDefinition())
	secrets := NewMemorySecretStore()
	secret := []byte("inbound-secret-value-32-bytes-long")
	if err := secrets.Create(context.Background(), "inbound.test", SecretMaterial{
		Revision: "r1", Value: secret, NotBefore: now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewMemory(catalog, MemoryOptions{
		Clock: func() time.Time { return now }, Secrets: secrets, Authorizer: testAuthorizer(now),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := encodeCloudEvent(catalog, "event-1", EventCommand{
		Type: "com.yueli.test.created.v1", Data: json.RawMessage(`{"id":"1"}`), OccurredAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{}
	headers.Set(HeaderWebhookID, "event-1")
	headers.Set(HeaderWebhookTimestamp, strconv.FormatInt(now.Unix(), 10))
	headers.Set(HeaderWebhookSignature, SignV1("event-1", now, body, secret))
	verified, err := runtime.Verify(context.Background(), "test.sender", IncomingMessage{
		Headers: headers, Body: body, ReceivedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := runtime.Accept(context.Background(), verified)
	if err != nil || !first.FirstSeen {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := runtime.Accept(context.Background(), verified)
	if err != nil || second.FirstSeen {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	modified := verified
	modified.bodyDigest = digestBytes([]byte("different"))
	if _, err := runtime.Accept(context.Background(), modified); !IsCode(err, ErrorIdempotency) {
		t.Fatalf("receipt conflict err=%v", err)
	}
}
