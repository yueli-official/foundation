package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"sync"
	"testing"
	"time"
)

type scriptedSender struct {
	mu       sync.Mutex
	statuses []int
	now      *time.Time
	headers  []http.Header
}

func (sender *scriptedSender) Send(_ context.Context, request OutboundRequest) (SendResult, error) {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	status := sender.statuses[0]
	sender.statuses = sender.statuses[1:]
	sender.headers = append(sender.headers, request.Header.Clone())
	return SendResult{
		StatusCode: status, StartedAt: *sender.now, FinishedAt: *sender.now,
		ResponseDigest: digestBytes(nil),
	}, nil
}

func TestDeliveryDriverRetriesWithStableEventIDAndNewTimestamp(t *testing.T) {
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
		ID: "driver", URL: "https://example.com/driver",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.PutSubscription(context.Background(), PutSubscriptionCommand{
		ID: "driver-events", EndpointID: endpoint.Endpoint.ID,
		EventTypes: []EventType{"com.yueli.test.created.v1"}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	event, err := runtime.Publish(context.Background(), EventCommand{
		Type: "com.yueli.test.created.v1", Data: json.RawMessage(`{"id":"driver"}`),
		OccurredAt: now, IdempotencyKey: "driver-event",
	})
	if err != nil {
		t.Fatal(err)
	}
	sender := &scriptedSender{statuses: []int{503, 204}, now: &now}
	driver := &DeliveryDriver{
		Backend: runtime, Secrets: secrets, Authorizer: testAuthorizer(now), Sender: sender,
		Retry: catalog.retry, Limits: catalog.limits, Clock: func() time.Time { return now },
	}
	deliveryID := scheduler.items[0].DeliveryID
	retrying, err := driver.Advance(context.Background(), deliveryID)
	if err == nil || retrying.State != DeliveryRetrying {
		t.Fatalf("retrying=%+v err=%v", retrying, err)
	}
	now = now.Add(time.Minute)
	driver.Authorizer = testAuthorizer(now)
	delivered, err := driver.Advance(context.Background(), deliveryID)
	if err != nil || delivered.State != DeliveryDelivered {
		t.Fatalf("delivered=%+v err=%v", delivered, err)
	}
	if len(sender.headers) != 2 {
		t.Fatalf("requests=%d", len(sender.headers))
	}
	if sender.headers[0].Get(HeaderWebhookID) != string(event.EventID) ||
		sender.headers[1].Get(HeaderWebhookID) != string(event.EventID) {
		t.Fatalf("webhook ids changed: %q %q", sender.headers[0].Get(HeaderWebhookID), sender.headers[1].Get(HeaderWebhookID))
	}
	if sender.headers[0].Get(HeaderWebhookTimestamp) == sender.headers[1].Get(HeaderWebhookTimestamp) {
		t.Fatal("attempt timestamp was not refreshed")
	}
	attempts, err := runtime.Attempts(context.Background(), deliveryID)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("attempts=%+v err=%v", attempts, err)
	}
}

type failingResolver struct{ err error }

func (resolver failingResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return nil, resolver.err
}

func TestDeliveryDriverRetriesTransientDNSButPermanentlyRejectsGoneEndpoint(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	catalog := MustCompile(testDefinition())
	scheduler := &captureScheduler{}
	runtime, err := NewMemory(catalog, MemoryOptions{
		Clock: func() time.Time { return now }, Scheduler: scheduler,
		Authorizer: testAuthorizer(now),
	})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := runtime.PutEndpoint(context.Background(), PutEndpointCommand{
		ID: "outcome", URL: "https://example.com/outcome",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.PutSubscription(context.Background(), PutSubscriptionCommand{
		ID: "outcome-events", EndpointID: endpoint.Endpoint.ID,
		EventTypes: []EventType{"com.yueli.test.created.v1"}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Publish(context.Background(), EventCommand{
		Type: "com.yueli.test.created.v1", Data: json.RawMessage(`{"id":"dns"}`),
		OccurredAt: now, IdempotencyKey: "outcome-dns",
	}); err != nil {
		t.Fatal(err)
	}
	sender := &scriptedSender{statuses: []int{http.StatusGone}, now: &now}
	driver := &DeliveryDriver{
		Backend: runtime, Secrets: runtime.secrets,
		Authorizer: NetworkAuthorizer{
			Resolver: failingResolver{err: errors.New("temporary DNS failure")},
			Policy:   PublicNetworkPolicy(), Clock: func() time.Time { return now },
		},
		Sender: sender, Retry: catalog.retry, Limits: catalog.limits,
		Clock: func() time.Time { return now },
	}
	deliveryID := scheduler.items[0].DeliveryID
	retrying, err := driver.Advance(context.Background(), deliveryID)
	if err == nil || retrying.State != DeliveryRetrying || retrying.LastErrorCode != "dns" {
		t.Fatalf("DNS outcome=%+v err=%v", retrying, err)
	}
	now = now.Add(time.Minute)
	driver.Authorizer = testAuthorizer(now)
	failed, err := driver.Advance(context.Background(), deliveryID)
	if err == nil || failed.State != DeliveryFailed || failed.LastErrorCode != "http_410" {
		t.Fatalf("gone outcome=%+v err=%v", failed, err)
	}
	if runtime.endpoints[endpoint.Endpoint.ID].current.State != EndpointDisabled {
		t.Fatal("410 did not disable endpoint")
	}
}

func TestDeliveryDriverAppliesRetryBudgetToSecretFailures(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	definition := testDefinition()
	definition.Retry.MaxAttempts = 1
	catalog := MustCompile(definition)
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
		ID: "secret-budget", URL: "https://example.com/secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.PutSubscription(context.Background(), PutSubscriptionCommand{
		ID: "secret-events", EndpointID: endpoint.Endpoint.ID,
		EventTypes: []EventType{"com.yueli.test.created.v1"}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Publish(context.Background(), EventCommand{
		Type: "com.yueli.test.created.v1", Data: json.RawMessage(`{"id":"secret"}`),
		OccurredAt: now, IdempotencyKey: "secret-budget",
	}); err != nil {
		t.Fatal(err)
	}
	if err := secrets.Delete(context.Background(), "webhook.endpoint.secret-budget", "r1"); err != nil {
		t.Fatal(err)
	}
	driver := &DeliveryDriver{
		Backend: runtime, Secrets: secrets, Authorizer: testAuthorizer(now),
		Sender: &scriptedSender{now: &now}, Retry: catalog.retry, Limits: catalog.limits,
		Clock: func() time.Time { return now },
	}
	failed, err := driver.Advance(context.Background(), scheduler.items[0].DeliveryID)
	if err == nil || failed.State != DeliveryFailed || failed.LastErrorCode != "retry_exhausted" {
		t.Fatalf("secret outcome=%+v err=%v", failed, err)
	}
}
