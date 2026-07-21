# Go Batch C: review and migration gate

The existing `platform/packages/go/gokit` is evidence, not a source directory to rename. Batch C redesigns a smaller ordinary-Go core and explicit GoFrame adapters under module `github.com/yueli-official/foundation/go`.

## Approved dependency direction

```text
contracts/http-problem
        |
        +-- problem       ordinary Go model, validation and JSON codec
        +-- auth          verifier policy and static key source
        +-- jwks          bounded HTTP fetch, cache and single-flight refresh
        +-- health        framework-free concurrent probe runner
        +-- telemetry     safe attributes and redaction policy
                 |
                 +-- goframe/http      Problem writer and trace adapter
                 +-- goframe/auth      request principal middleware
                 +-- goframe/health    HTTP/DB/Redis adapters
                 +-- goframe/openapi   explicit export command adapter
```

Core packages must not import GoFrame, read process environment, install process-global providers, or know Yueli deployment names. GoFrame adapters may depend on core packages; the reverse dependency is forbidden.

## Findings that block a direct copy

- `authjwt.RemoteKeySource` performs the remote JWKS request while holding one mutex. A slow issuer serializes all verifier calls. It also needs explicit URL/scheme, redirect, response-size, timeout, stale-key and refresh-error policy.
- `errs` uses a mutable process-global code/status registry. Registration can silently overwrite a code and is not concurrency-safe.
- `ghttpx` creates a process-global rate limiter from `PLATFORM_RATE_LIMIT_PER_MINUTE` during package initialization. Client identity also depends on undeclared trusted-proxy topology.
- `response.Envelope` forces success and failure through `Data any` and carries a raw message. Batch A instead fixed raw success DTOs plus strict RFC 9457 Problem failures and caller-owned i18n keys/params.
- `healthcheck` mixes a valuable probe runner with GoFrame globals, HTTP rendering and logging.
- `observability` mixes reusable redaction with `PLATFORM_*` environment policy and process-global telemetry setup.
- `mail`, `capability` and `classification` have different domain/security lifecycles and are not part of the foundation HTTP/Auth batch.

## Execution slices

1. **Problem core:** implement immutable Problem kinds, safe JSON parameters, violations and canonical schema fixture conformance. No global registry and no success envelope.
2. **GoFrame Problem adapter:** run a real loopback GoFrame server and verify content type, status, trace ID, malformed input and no diagnostic leakage against the same fixtures used by TypeScript.
3. **JWT/JWKS core:** separate verifier policy from transport. Add concurrent cache tests, single-flight refresh, stale-known-key behavior, unknown-kid throttling, bounded bodies, redirects disabled by default and context deadline propagation.
4. **Health core/adapters:** preserve panic isolation, deadline, sorted failures and in-flight protection in ordinary Go; inject database/Redis checks and HTTP representation in adapters.
5. **Telemetry/trace:** preserve the sanitizer only after a golden sensitive-attribute corpus passes; make providers, resource attributes, exporters and environment loading application-owned configuration.
6. **Consumer cutover:** migrate one small GoFrame service first, then a second service with different auth/health usage. Platform may use a temporary `replace`, but release conformance must install a tagged module without a local replace.

## Required gates

- `go test -race ./...`, `go vet ./...` and static/security scans;
- canonical valid, invalid and malformed Problem fixtures shared with the JS runtime;
- real GoFrame server tests, not request mocks only;
- JWKS concurrency and slow/failing issuer tests with leak-free shutdown;
- an external blank consumer installing a real module tag or local proxy artifact;
- two platform consumers before a package is promoted beyond experimental;
- no `platform/gokit`, `PLATFORM_*`, internal host, secret or mutable process-global policy in the published module.

The first implementation slice is Problem core plus GoFrame Problem adapter. Auth/JWKS starts only after the wire contract passes cross-language conformance.

## Current status

The first slice is implemented in `go/problem` and `go/goframe/http`. Canonical valid/invalid fixtures, malformed/trailing JSON, immutable Kind construction, JSON-safe caller parameters, bounded response serialization and a real loopback GoFrame response contract pass `go test -race ./...` and `go vet ./...`.

The JWKS transport slice is implemented in `go/jwks` as a redesign, not a copy. Its public Interface is a small context-aware key source; the remote implementation performs no network work while holding its state lock. Refresh work is single-flight and timeout-bounded, redirects are disabled, bodies are bounded, HTTP is restricted to an explicit loopback-only test/development option, unknown key IDs are throttled after refresh completion, and known stale keys remain available during issuer failure. Concurrent initial load, slow issuer, failed issuer, key rotation, redirect, oversized response and caller-cancellation behavior pass race tests.

The JWT policy slice is implemented separately in `go/auth`. It accepts a consumer-owned one-method `KeySource`, requires `exp` and an actor by default, uses an explicit asymmetric algorithm allowlist, optionally enforces `typ`, any-match audiences and maximum token lifetime, and bounds compact token size before parsing. Signature verification is separated from typed claim decoding so malformed claims cannot be misreported as bad signatures. The old verifier's acceptance of access tokens without expiry is intentionally removed. HTTP bearer parsing, error rendering and request-context storage remain adapter responsibilities.

The first GoFrame auth adapter is implemented in `go/goframe/auth` and verified through a real loopback server. It rejects multiple or malformed Authorization headers, renders caller-owned RFC 9457 Problems with an RFC 6750 challenge, never exposes verifier diagnostics, and stores the verified Principal through the ordinary-Go auth context helpers. Optional authentication permits a missing credential but intentionally rejects an invalid supplied credential; the old silent authenticated-to-anonymous downgrade is removed.
