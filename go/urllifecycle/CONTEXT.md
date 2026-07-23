# URL Lifecycle context

## Language

- **ResourceKey**: stable product entity identity (`kind + id`).
- **RouteKey**: one stable public representation (`ResourceKey + variant`).
- **LocalRef**: normalized origin-relative path plus registered identity query.
- **Canonical**: the current primary LocalRef for one RouteKey.
- **Alias**: a Route-owned alternate entry that follows its current canonical.
- **Redirect**: a historical or administrative outcome with explicit status,
  query behavior, and target.
- **Gone**: retained knowledge that a former public URL is intentionally and
  probably permanently unavailable.
- **Unknown**: no lifecycle record; distinct from Gone.
- **Temporary overlay**: a 302/307 redirect over an owned base outcome that
  reveals the base again after removal/expiry.
- **ChangeSet**: unordered, declarative final-state transition.
- **Revision**: instance-wide serial URL mutation number.

## Ownership

- Foundation owns URL normalization, query identity, state transitions,
  conflicts, terminal targets, revision/idempotency, audit/archive, PostgreSQL
  schema, and HTTP protocol mapping.
- Products own content entities, hierarchy/path derivation, publication state,
  authorization, actor, reason, and retirement choice.
- Discovery consumes committed canonical claims only.
- PostgreSQL truth stays in each independent consumer instance. There is no
  remote Store or multi-instance shared database.

## Frozen policy

- RFC-compatible path normalization decodes percent-encoded unreserved bytes,
  uppercases remaining percent triplets, removes dot segments, preserves path
  case/repeated slash/trailing slash, and rejects malformed encoding, NUL,
  controls, backslash, query, fragment, authority, and scheme-relative input.
- Query identity is a compiled, ordered, single-value schema. Unregistered
  request query remains outside URL ownership.
- `CanonicalWithExtras` uses target canonical identity and preserves only
  non-identity source query. Preserve, Drop, and Replace remain explicit.
- 301/302 are explicit legacy method-changing compatibility choices. 307/308
  preserve method and are the temporary/permanent defaults. 303 is excluded.
- Internal targets are RouteKeys. Normal public resolution does not traverse a
  redirect graph.
- External targets require an exact compiled origin allowlist and never derive
  from Host, forwarded headers, or request query.
- 301/308/404/410 heuristic cacheability is never relied upon; the HTTP Adapter
  emits explicit policy.

## Persistence

PostgreSQL current-state tables are request-path truth. Command receipts and
history are append-only evidence. Export/restore preserves routes, references,
overlays, gone, commands, revisions, and optional audit.

The instance revision row serializes URL writes. Resolve uses indexed reference
and target-route lookups and does not acquire the write lock.

The caller-owned `*sql.Tx` binding is the real transaction Seam. It allows
product rows, URL state, and product audit/outbox to commit together while the
Adapter retains all URL invariants.
