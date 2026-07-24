# Webhook context

## Owns

- instance-local Event and exact CloudEvents body;
- Endpoint and Subscription revisions and current safety state;
- Event-to-Subscription fan-out and Delivery truth;
- Standard Webhooks signing profile and secret revision evidence;
- HTTP response classification, Attempt evidence, dead delivery and replay;
- inbound Standard Webhooks verification and minimal replay receipt;
- Webhook schema, Memory/PostgreSQL adapters and Work handler.

## Does not own

- product event field meaning or schema evolution decisions;
- product/provider callback normalization and business state;
- payment amount/order/merchant validation or provider acknowledgement shape;
- Work leases, polling, process concurrency or general background work;
- Authorization roles, Audit journal, metrics transport or secret storage;
- cross-instance event routing or a central integration service.

## Neighbor contracts

- Product code writes its truth and calls `PublishTx` in the same transaction.
- Work receives only `DeliveryID` and executes the handler. Delivery remains
  Webhook truth even if a Work job reaches terminal state.
- SecretStore supplies primary/previous bytes. Webhook persists only revision
  identifiers.
- Authorization and Audit protect/record Endpoint, Subscription, rotation and
  replay management at the application boundary.
- Privacy may govern product payload minimization/retention; it does not route
  events.

## Exactness and crash boundaries

Local publication is atomic with caller data when `PublishTx` is used. Remote
delivery is at least once. A process crash after remote acceptance but before
Attempt completion leaves an unknown Attempt and Work may send the same Event
again. Stable Event ID and exact body are the receiver dedupe contract.

Inbound verification is not durable dedupe. `AcceptTx` must share the transaction
that applies the provider/product fact. A rollback removes both receipt and
product effects.

## Security boundary

Endpoint configuration is untrusted administrative input. URL/DNS checks occur
when saving and before every attempt. The authorized address is pinned into the
dial while Host/TLS SNI retain the original hostname. Redirects, ambient proxies,
userinfo, unsafe address classes and unbounded responses are denied.

Raw request bodies are signed but are not copied to Attempt or inbound receipt.
Secret and signature values are never persisted.

## Evolution

CloudEvents binary mode, Standard Webhooks Ed25519, RFC 9421, KMS/Vault stores
and provider-native verifier adapters are additional adapters/profiles. They
must preserve Event/Delivery/Attempt/Receipt semantics and must not enlarge the
ordinary Publisher interface.
