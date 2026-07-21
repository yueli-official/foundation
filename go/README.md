# Go modules

Go migration is intentionally review-first. Existing `platform/gokit` packages are evidence, not a directory blueprint.

Initial candidates:

- ordinary Go Problem/wire contracts;
- JWT/JWKS verification core;
- health probe runner;
- observability context;
- explicit GoFrame response, auth, health and OpenAPI adapters.

No implementation is copied until its Interface, dependency direction, concurrency behavior and conformance tests are approved.

The approved findings, package graph, execution slices and release gates are recorded in [Go Batch C](../docs/go-batch-c.md). The first implementation slice is the ordinary-Go Problem core plus a real GoFrame Problem adapter.
