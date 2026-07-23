# Traffic

`github.com/yueli-official/foundation/go/traffic` is a public, ordinary-Go
module for privacy-bounded view counters in independently deployed products.
It records typed resource views, exact event replay protection, daily visitor
deduplication, totals, daily series and top-resource queries.

The module is deliberately smaller than a general analytics platform. It does
not model sessions, funnels, conversions, arbitrary event names, referrers,
campaign attribution or cross-day people.

## Semantics

- `EventID` is delivery idempotency. Replaying the same ID and payload returns
  `Replay=true` without another increment. Reusing the ID for a different
  payload is a conflict.
- A counted observation always adds one view. A supplied visitor contributes
  at most one unique visitor per local calendar day at instance scope and once
  per visited resource on that day.
- `UniqueVisitorDays` is the sum of daily unique visitors. Across a multi-day
  range it is not a cross-day distinct-person count.
- `DateRange` is half-open: `From` is included and `To` is excluded.
- Unknown and human visits count by default. Bot and internal visits still get
  idempotency receipts but are dropped from aggregates.
- Baseline imports add historical all-time views only. They cannot fabricate
  daily history or historical unique visitors.

## Consumer definition

Each deployment compiles an immutable catalog. Resource kinds and the IANA time
zone are consumer vocabulary, not package defaults:

```go
catalog := traffic.MustCompile(traffic.Definition{
    Version:  traffic.DefinitionVersion,
    TimeZone: "Asia/Shanghai",
    ResourceKinds: []traffic.ResourceKindDefinition{
        {Key: "post"},
        {Key: "page"},
    },
})
```

Changing the compiled version, digest or time zone for an existing PostgreSQL
instance fails startup. Treat those values as persisted compatibility
contract, and deploy an explicit migration for a real definition change.

## Recording

Generate the event ID in the browser before delivery and keep the timestamp and
ID unchanged when retrying. Resolve the resource and classify the request in
trusted server code:

```go
occurredAt := time.Now().UTC() // or a validated browser timestamp retained on retry
seed := []byte("subject\x00" + verifiedSubject)
token, err := module.TokenizeVisitor(ctx, occurredAt, seed)
if err != nil { /* fail or deliberately omit unique counting */ }

result, err := module.Record(ctx, traffic.Observation{
    EventID:      traffic.EventID(browserEventID),
    Resource:     traffic.Resource{Kind: "post", ID: postID},
    OccurredAt:   occurredAt,
    Class:        traffic.VisitHuman,
    HasVisitor:   true,
    VisitorToken: token,
})
```

For an anonymous visitor, a consumer may use a short-lived server-side seed
such as a trusted client IP plus User-Agent. The module immediately HMACs it
with an instance secret and calendar day. Never put raw IP, User-Agent,
account ID or email in `VisitorToken`, a resource ID or an event ID. The module
does not persist the seed.

Consent, Global Privacy Control, Do Not Track, trusted-proxy selection,
internal-network classification and bot rules are deployment policy. A
consumer can omit the visitor token, classify the visit as bot/internal, or
skip `Record` entirely. Keep that choice outside this transport-neutral core.

`RecordBatch` is atomic and preserves input order. It is useful for a trusted
collector, not for mixing unrelated browser requests.

## Queries

```go
summary, _ := module.Summary(ctx, traffic.SummaryQuery{
    Scope: traffic.ResourceScope(traffic.Resource{Kind: "post", ID: postID}),
})

series, _ := module.Series(ctx, traffic.SeriesQuery{
    Scope: traffic.InstanceScope(),
    Range: traffic.DateRange{
        From: traffic.MustParseDay("2026-07-01"),
        To:   traffic.MustParseDay("2026-08-01"),
    },
})

top, _ := module.Top(ctx, traffic.TopQuery{
    ResourceKind: "post",
    Metric:       traffic.RankViews,
    Limit:        20,
})
```

`Totals` is the projection-reconciliation API and preserves caller order,
including zero totals. It accepts at most 1,000 resources per call.

## PostgreSQL

The PostgreSQL adapter stores truth inside the consumer database. It needs no
Redis, Kafka, ClickHouse, watcher or remote service:

```go
db, _ := sql.Open("postgres", consumerDSN)
module, err := trafficpostgres.New(ctx, catalog, trafficpostgres.Options{
    DB:          db,
    InstanceKey: "blog:example",
})
```

Applications do not auto-migrate. Generate immutable migrations into each
consumer repository:

```sh
go run ./traffic/postgres/cmd/trafficschema \
  -dir ./manifest/sql/migrations \
  -name 0014_traffic_v1
```

The generator records a canonical digest and refuses to overwrite drifted
files.

For a legacy counter, read all old values before adapter construction and pass
them through `InitialBaselines`. Instance creation and all initial imports then
commit in one transaction:

```go
module, err := trafficpostgres.New(ctx, catalog, trafficpostgres.Options{
    DB: db, InstanceKey: "blog:example",
    InitialBaselines: []traffic.BaselineImport{{
        Source: "blog.post_stats.view_count",
        Resource: traffic.Resource{Kind: "post", ID: postID},
        Views: legacyViews,
    }},
})
```

Initial baselines are ignored after the instance exists. `ImportBaseline`
remains available for explicit later backfills; the same source/resource/value
replays, while changing that tuple conflicts.

## Projection and failure policy

Product lists often need a local `view_count` for joins and sorting. Keep it as
a derived projection, not a second source of truth:

1. Record in Traffic.
2. Set the product projection to
   `GREATEST(current_value, result.ResourceTotals.Views)`.
3. On startup, call `Totals` and replace product projections exactly before
   accepting HTTP traffic.

If the module write fails, do not update the projection. If the module commits
but projection update fails, return an error and retry the identical event;
the module replay returns its current total and repairs the projection.

Traffic persistence is normally a readiness dependency. Silently falling back
to an in-memory counter creates irreconcilable truth and is not supported.

## Retention and deletion

Defaults accept events up to seven days old, retain event receipts for seven
days, and retain visitor markers for nine days. `Prune(now)` removes expired
receipts and visitor markers; totals and daily aggregates remain. Schedule it
at least daily and alert on failures.

After receipt pruning, a very late replay can count again, which is why receipt
retention must be at least the accepted event age. Visitor marker retention is
longer than accepted event age by at least one day so a valid late event cannot
inflate a daily unique count.

`ForgetResource` removes resource-scoped totals, daily rows, receipts, markers
and baselines. It intentionally preserves instance aggregates because an
instance unique visitor cannot be safely subtracted after visiting multiple
resources. Whether ordinary product deletion preserves historical analytics or
invokes this privacy operation is consumer policy.

## Verification

Every adapter should run `traffictest.Run`. The reference Memory and PostgreSQL
adapters share the suite for replay, concurrency, visitor dedup, query,
baseline, pruning and deletion behavior. PostgreSQL tests additionally cover
schema drift, restart, atomic initial baselines and indexed scale queries.

Blog and Resource are structurally different production consumers: Blog keeps
`post_stats` as a joined projection; Resource migrates a previously editable
column to a read-only projection.
