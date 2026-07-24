# Go Foundation

Module path: `github.com/yueli-official/foundation/go`.

The module publishes ordinary-Go cores and explicit GoFrame adapters. It does
not read company environment variables, install process globals at import time,
or wrap successful endpoint DTOs in a generic envelope.

## Packages

- `problem`: strict RFC 9457 wire value, immutable `Kind`, immutable application
  `Error`, validation violations, bounded codec and safe error mapping.
- `goframe/http`: bounded Problem writer and GoFrame error adapter. Successful
  responses remain endpoint-owned raw JSON DTOs.
- `httpclient`: bounded raw-success decoder that requires
  `application/problem+json` for non-2xx responses.
- `health`: concurrent ordinary-Go runner with deadlines, stable snapshots,
  panic isolation and one in-flight call per non-cooperative check.
- `goframe/health`: GoFrame ready/liveness representation adapter.
- `jwks`: bounded HTTPS JWKS resolution, single-flight refresh, stale-known-key
  policy and unknown-key throttling.
- `auth` and `goframe/auth`: typed principals, strict JWT policy and required or
  optional GoFrame bearer middleware.
- `authorization` and `authorization/postgres`: public instance-local
  Capability/Role/Scope core, typed Constraints and Query planning, complete
  role/workflow/policy management, normalized PostgreSQL truth, a derived
  Casbin v3 projection, schema generator, decision/management audit, offline
  recovery and reusable Adapter conformance. Docs and Navigation exercise the
  public Interface with different scope and delegation models.
- `audit` and `audit/otelmirror`: typed instance-local management and security
  evidence, caller-owned PostgreSQL transaction binding, bounded keyset reads,
  streaming export, legal hold, archive-before-purge retention, tamper-evident
  sequence verification and durable committed-event mirrors.
- `search`: instance-local searchable projections, revision-safe changes,
  declared analyzers/filters/facets, structured highlights, keyset cursors,
  caller-owned PostgreSQL transactions and generation rebuilds.
- `traffic` and `traffic/postgres`: privacy-bounded typed resource views,
  exact event replay protection, daily instance/resource visitor deduplication,
  half-open range queries, atomic legacy baselines, retention maintenance,
  immutable schema generation and shared Adapter conformance. Blog and Resource
  exercise joined and inline read-projection migrations.
- `work` and `work/postgres`: reliable instance-local background jobs,
  transactional enqueue/outbox, delayed and recurring schedules, queue
  concurrency, leases/heartbeats, retry, pause/cancel, progress, immutable
  attempts, replay, retention, schema generation and shared Adapter
  conformance.
- `abuse` and `abuse/turnstile`: stable bound external-input Actions,
  instance-local atomic multi-budget admission, pending/committed outcome
  penalties, purpose-bound Signal pseudonyms, explicit challenge/reject
  decisions, Module-owned PostgreSQL transactions, governance/retention and
  server-side provider verification.
- `privacy`: typed instance-local Processing Purpose decisions, immutable
  consent/withdrawal/signal evidence, calendar retention reviews, verified
  Rights Request orchestration and an idempotent remote data Owner protocol.
  Identity may coordinate, but each product retains source-data and disposition
  ownership.
- `discovery`: trusted-origin page projection and atomic publication for
  canonical/robots, Open Graph/X Card, typed JSON-LD, streaming sitemap/index,
  RSS/Atom and robots.txt. Product cursor sources and publication targets remain
  instance-local; a versioned contract feeds the Nuxt Adapter.
- `urllifecycle`, `urllifecycle/postgres` and `urllifecycle/httpadapter`:
  instance-local canonical Route claims, query-aware variants, aliases,
  301/302/307/308 redirects, temporary overlays, 410 Gone, atomic declarative
  transitions, revision/idempotency, archive/recovery, caller-owned PostgreSQL
  transactions and trusted-origin HTTP resolution.
- `siteprofile` and `siteprofile/httpadapter`: typed instance-local public site
  identity, branding Asset/icon references, scheduled announcement, support,
  footer, social/legal/compliance material, conditional revision/ETag,
  schema-driven management metadata, caller-owned PostgreSQL transactions and
  conditional HTTP reads/writes.
- `telemetry`: mandatory pre-export secret/SQL/body/full-URL sanitizer,
  low-cardinality server span naming, explicit provider assembly and idempotent
  HTTP client instrumentation.
- `goframe/ratelimit`: bounded fixed-window state with explicit policy and
  caller-owned client-key/trusted-proxy selection.
- `goframe/openapi`: explicit route-derived OpenAPI export with opt-in overwrite;
  it never reads environment variables.

## HTTP contract

Handlers return their concrete success type. Failures use a stable machine code
and a caller-owned public type URI:

```go
kind := problem.MustKind("identity.not_authenticated", http.StatusUnauthorized)
failure, err := problem.New(kind,
    "https://errors.example.test/problems/identity.not_authenticated",
    traceID,
    nil,
)
if err != nil { /* configuration error */ }

writer := foundationhttp.MustWriter(foundationhttp.WriterOptions{TraceHeader: "X-Trace-Id"})
if err := writer.Write(request, failure); err != nil { /* transport failure */ }
```

Consumers decode the same contract without learning an envelope shape:

```go
profile, err := httpclient.DecodeJSON[Profile](response, httpclient.Limits{
    SuccessBytes: 1 << 20,
    ProblemBytes: 64 << 10,
})
```

`RemoteError` exposes only the structured public Problem. Transport diagnostics
remain server-side.

## Explicit process policy

Applications construct deployment policy at their entry point. For telemetry,
the application creates the exporter, sampler and any company resource
attributes; Foundation always wraps the exporter with its privacy boundary:

```go
provider, err := telemetry.NewProvider(ctx, telemetry.Config{
    ServiceName: "example-api",
    Exporter: exporter,
    Sampler: sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1)),
})
if err != nil { /* fail startup */ }
if err := telemetry.InstallGlobal(provider); err != nil { /* fail startup */ }
defer telemetry.ShutdownWithTimeout(provider.Shutdown, 5*time.Second)
```

Rate limiting likewise separates reusable state from topology. The GoFrame
adapter evaluates a caller-provided key; selecting forwarded IPs is an
application decision made only after trusted proxies are configured.

OpenAPI export receives `Server`, `Output` and `Overwrite` through
`openapi.ExportConfig`. Environment/CLI parsing belongs in the binary.

## Verification

```sh
go test -race ./...
go vet ./...
```

The suite includes real loopback GoFrame servers, concurrent and non-cooperative
health probes, malformed/trailing/oversized HTTP bodies, JWKS failures and a
telemetry redaction corpus. See [Go Batch C](../docs/go-batch-c.md) and
[local consumption](../docs/local-consumption.md).
