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
