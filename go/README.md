# Go modules

Go migration is intentionally review-first. Existing `platform/gokit` packages are evidence, not a directory blueprint.

Approved package direction:

- `problem`: ordinary Go Problem/wire contracts;
- `jwks`: hardened static and remote public-key resolution;
- `auth`: JWT verification policy (next slice);
- health probe runner;
- observability context;
- explicit GoFrame response, auth, health and OpenAPI adapters.

No implementation is copied until its Interface, dependency direction, concurrency behavior and conformance tests are approved. `jwks.RemoteSource` uses HTTPS by default, disables redirects, bounds response size and refresh duration, coalesces concurrent refreshes, throttles unknown key IDs and preserves known stale keys during issuer failure. It has no GoFrame, environment, process-global or platform observability dependency.

The approved findings, package graph, execution slices and release gates are recorded in [Go Batch C](../docs/go-batch-c.md). Implemented slices are the ordinary-Go Problem core, real GoFrame Problem adapter and JWKS key source. JWT claims/algorithm verification remains a separate pending module so transport policy cannot silently become authentication policy.
