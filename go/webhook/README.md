# Webhook

`webhook` is a public, framework-neutral, instance-local module for durable
outbound webhooks and minimal inbound replay protection.

It combines:

- CloudEvents 1.0 structured JSON bodies;
- Standard Webhooks v1 HMAC-SHA256 signatures;
- immutable Event, Endpoint/Subscription revision, Delivery and Attempt truth;
- caller-owned PostgreSQL transactions;
- Foundation Work execution, retry and crash recovery;
- per-attempt DNS/SSRF authorization and pinned HTTP connections;
- secret overlap rotation, dead delivery and linked replay;
- minimal inbound verification receipts.

It is not a central event bus, cross-site database, workflow engine or provider
callback rule set. Each consumer instance owns its tables and product event
schemas.

## Domain publish

Compile the code-owned event vocabulary at startup:

```go
catalog := webhook.MustCompile(webhook.Definition{
	Version:  webhook.DefinitionVersion,
	Consumer: "commerce",
	Source:   "urn:yueli:shop:main:commerce",
	EventTypes: []webhook.EventTypeDefinition{{
		Type:         "com.yueli.commerce.order.fulfilled.v1",
		MaxDataBytes: 64 << 10,
	}},
})
```

Product code publishes a fact, never a URL:

```go
receipt, err := hooks.Publish(ctx, webhook.EventCommand{
	Type:           "com.yueli.commerce.order.fulfilled.v1",
	Subject:        "order/" + order.ID,
	Data:           payload,
	OccurredAt:     order.FulfilledAt,
	IdempotencyKey: "order:" + order.ID + ":fulfilled:v1",
})
```

`Publish` means local durable acceptance. It does not wait for a remote endpoint.
Delivery is explicitly at least once.

When the fact follows a product mutation, bind to the caller transaction:

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil { return err }
defer tx.Rollback()

if err := settleOrder(ctx, tx, order); err != nil { return err }
if _, err := hooks.PublishTx(ctx, tx, command); err != nil { return err }
return tx.Commit()
```

The product change, Event, fan-out Deliveries and Work intents commit or roll
back together.

## Endpoint and subscription administration

Endpoint URLs and subscriptions are management data. They use optimistic ETags
and immutable revisions:

```go
credential, err := hooks.PutEndpoint(ctx, webhook.PutEndpointCommand{
	ID:  "erp",
	URL: "https://erp.example.com/hooks/commerce",
})
// credential.Secret is returned only on create or rotate.

_, err = hooks.PutSubscription(ctx, webhook.PutSubscriptionCommand{
	ID:          "erp-orders",
	EndpointID:  credential.Endpoint.ID,
	EventTypes:  []webhook.EventType{"com.yueli.commerce.order.fulfilled.v1"},
	Enabled:     true,
})
```

The application must protect these methods with its Authorization module and
record management changes through Audit. The package does not infer HTTP roles.

`paused` stops and can explicitly resume pending delivery. `disabled` cancels
pending delivery. `revoked` is irreversible for that Endpoint identity. A 410
response creates a disabled Endpoint revision.

## Work integration

Create a separate Work catalog/instance so product Work definition changes do
not drift the webhook executor:

```go
workCatalog := work.MustCompile(work.Definition{
	Version: work.DefinitionVersion,
	Queues: []work.QueueDefinition{{Key: "webhook", Concurrency: 4}},
	Kinds: []work.KindDefinition{webhook.WorkDefinition("webhook")},
})

scheduler := &workadapter.Adapter{Work: workPostgres}
handler := webhook.NewWorkHandler(driver)
```

The Work payload contains only a `DeliveryID`. Webhook loads the immutable body
and route snapshot, resolves current secrets, authorizes fresh DNS answers,
signs and sends. Work owns leasing and retry timing; Webhook owns delivery and
HTTP outcome semantics.

## Protocol

Each request sends:

```text
Content-Type: application/cloudevents+json
Webhook-Id: <EventID>
Webhook-Timestamp: <attempt unix seconds>
Webhook-Signature: v1,<base64 HMAC-SHA256>
Webhook-Delivery-Id: <DeliveryID>
```

The signed bytes are exactly:

```text
<webhook-id>.<webhook-timestamp>.<raw body>
```

The CloudEvent `id` and `Webhook-Id` are the same stable Event ID. Automatic
retry and manual replay preserve the Event ID and exact body. Replay creates a
new Delivery lineage and uses current secret/network policy; it does not bypass
receiver idempotency.

Secret rotation has one primary and a bounded previous revision. Both may sign
during overlap. Secret bytes belong to the injected `SecretStore` and never
enter definitions, PostgreSQL rows, errors, logs, metrics or Audit.

`PostgresSecretStore` is the production-ready local adapter. It encrypts each
revision with AES-256-GCM, binds ciphertext to instance/reference/revision as
associated data and requires a 32-byte deployment master key. The master key is
never persisted. A Vault/KMS adapter can implement the same `SecretStore`
interface when the deployment already owns such a system.

## Network policy

The production default:

- accepts only HTTPS port 443;
- rejects userinfo, fragments and ambient proxy credentials;
- validates every DNS answer at Endpoint save and before every attempt;
- rejects loopback, private, link-local, multicast, unspecified, carrier-grade
  NAT, documentation and metadata ranges;
- pins the approved IP while preserving the original Host and TLS ServerName;
- disables redirects and response decompression;
- bounds DNS, connect, TLS, response header/body and total deadlines.

Private/HTTP development policy must be injected explicitly. It is never a
per-publish flag.

## Delivery outcomes

- 2xx: delivered.
- 408, 425, 429 and 5xx: retry within attempt/age policy.
- valid `Retry-After`: honored only within the configured maximum.
- 410: failed and Endpoint disabled.
- other 3xx/4xx: permanent failure.
- transient DNS/connect/TLS/timeout: retry.
- SSRF/security denial: permanent failure.

A remote endpoint may commit and lose the response. The resulting retry is
unavoidable; receivers must deduplicate the stable Event ID.

Manual `Replay` accepts terminal failed/cancelled delivery, requires a reason
and idempotency key, creates a new Delivery and preserves all original history.

## Inbound verification

Inbound source definitions bind an expected CloudEvents source, allowed event
types, secret reference, body limit and mandatory timestamp window.

```go
verified, err := hooks.Verify(ctx, "partner.primary", webhook.IncomingMessage{
	Headers:    request.Header,
	Body:       rawBody,
	ReceivedAt: receivedAt,
})
if err != nil { return reject(err) }

tx, err := db.BeginTx(ctx, nil)
if err != nil { return err }
defer tx.Rollback()

receipt, err := hooks.AcceptTx(ctx, tx, verified)
if err != nil { return err }
if receipt.FirstSeen {
	if err := applyPartnerFact(ctx, tx, verified.Body()); err != nil { return err }
}
return tx.Commit()
```

Verification is pure. `AcceptTx` makes replay protection and the product mutation
one atomic commit. The receipt stores IDs, digests, verified secret revision and
times, not raw bodies or arbitrary headers.

Provider-native callbacks remain product adapters. Alipay/WeChat payment
verification and provider acknowledgements do not become Standard Webhooks.

## PostgreSQL schema

Generate an immutable migration:

```sh
go run ./webhook/cmd/webhookschema \
  -dir ./manifest/migrations \
  -name 20260724_webhook_v1
```

The PostgreSQL adapter requires the generated Webhook migration and a separate
Foundation Work schema. It fails startup on schema/catalog drift.

## Verification

```sh
go test -race ./webhook/...
go vet ./webhook/...

WEBHOOK_POSTGRES_DSN='postgres://postgres:postgres@host:5432/postgres?sslmode=disable' \
  go test ./webhook -run Postgres -count=1
```

Tests cover signature vectors/raw-byte sensitivity, rotation, publish
idempotency/fan-out, replay lineage, inbound conflict, mixed/rebound DNS,
redirect prohibition, retry identity, transaction rollback, Work enqueue and
real PostgreSQL delivery.
