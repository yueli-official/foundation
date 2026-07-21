# Go modules

Go migration is intentionally review-first. Existing `platform/gokit` packages are evidence, not a directory blueprint.

Approved package direction:

- `problem`: ordinary Go Problem/wire contracts;
- `jwks`: hardened static and remote public-key resolution;
- `auth`: JWT access-token verification policy and typed Principal;
- health probe runner;
- observability context;
- explicit GoFrame response and auth adapters; health and OpenAPI adapters remain pending.

No implementation is copied until its Interface, dependency direction, concurrency behavior and conformance tests are approved. `jwks.RemoteSource` uses HTTPS by default, disables redirects, bounds response size and refresh duration, coalesces concurrent refreshes, throttles unknown key IDs and preserves known stale keys during issuer failure. It has no GoFrame, environment, process-global or platform observability dependency.

The approved findings, package graph, execution slices and release gates are recorded in [Go Batch C](../docs/go-batch-c.md). Implemented slices are the ordinary-Go Problem core, real GoFrame Problem adapter, JWKS key source, separate JWT verifier policy and GoFrame auth middleware. `auth.KeySource` is a one-method consumer-owned Interface that `jwks` sources satisfy structurally, keeping transport policy out of authentication policy.

The GoFrame auth adapter uses caller-owned Problem kind/type/trace policy. Required auth rejects missing credentials; optional auth permits absence but rejects malformed or invalid credentials rather than silently downgrading an authentication attempt to anonymous.
