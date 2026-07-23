# Work

`github.com/yueli-official/foundation/go/work` is a public, ordinary-Go Module
for reliable instance-local background work. It provides durable enqueue,
delayed and recurring jobs, bounded queue concurrency, leases and heartbeats,
retry, pause/cancel, progress, immutable attempt history, replay and retention.

It is not a shared scheduler service. Every independently deployed consumer
stores Work truth in its own PostgreSQL database and owns its job payloads,
handlers and side-effect idempotency.

## Definition

Compile the queues, kinds, retry policy and recurring schedules at startup:

```go
catalog := work.MustCompile(work.Definition{
    Version: work.DefinitionVersion,
    Queues: []work.QueueDefinition{
        {Key: "delivery", Concurrency: 8},
        {Key: "maintenance", Concurrency: 2},
    },
    Kinds: []work.KindDefinition{
        {
            Key: "notification.send", Queue: "delivery",
            DefaultAttempts: 5, MaxAttempts: 20, Timeout: 30 * time.Second,
        },
        {
            Key: "asset.rebuild", Queue: "maintenance",
            DefaultAttempts: 3, MaxAttempts: 10, Timeout: 2 * time.Hour,
        },
        {
            Key: "asset.prune", Queue: "maintenance",
            DefaultAttempts: 3, MaxAttempts: 10, Timeout: 2 * time.Hour,
        },
    },
    Schedules: []work.ScheduleDefinition{{
        Key: "asset-prune", Cron: "15 3 * * *", TimeZone: "Asia/Shanghai",
        Kind: "asset.prune",
    }},
})
```

The Definition is a persisted compatibility contract. PostgreSQL refuses to
start an existing instance if its Catalog version or digest differs. Deploy an
explicit migration for a real definition change.

Cron is a standard five-field expression. Parser types are not exposed.
Schedules use IANA time zones and materialize deterministic idempotency keys.

## Enqueue and transactional Outbox

Ordinary enqueue supports exact replay:

```go
result, err := module.Enqueue(ctx, work.Request{
    Kind:           "notification.send",
    Payload:        json.RawMessage(`{"messageId":"msg-1"}`),
    IdempotencyKey: "notification:msg-1",
})
```

The same idempotency key and normalized request returns the original Job with
`Replay=true`; changing the request conflicts.

When a domain mutation and its work must be atomic, use the PostgreSQL Adapter
inside the caller's transaction:

```go
err := goFrameDB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
    if _, err := tx.Model("messages").Data(message).Insert(); err != nil {
        return err
    }
    _, err := workAdapter.EnqueueTx(ctx, tx.GetSqlTX(), work.Request{
        Kind: "notification.send", Payload: payload,
        IdempotencyKey: "notification:" + message.ID,
    })
    return err
})
```

The caller owns commit/rollback. Foundation never starts a hidden second
transaction for `EnqueueTx`.

## Handlers and Runner

Handlers receive opaque consumer JSON and a lease-bound progress reporter:

```go
handlers := map[work.Kind]work.Handler{
    "notification.send": work.HandlerFunc(func(
        ctx context.Context, job work.Job, progress work.Progress,
    ) (work.Result, error) {
        if err := provider.Send(ctx, job.Payload); err != nil {
            if provider.IsPermanent(err) {
                return work.Result{}, work.Permanent(err)
            }
            return work.Result{}, work.RetryAfter(err, 30*time.Second)
        }
        return work.Result{Summary: "sent"}, nil
    }),
}

runner, err := work.NewRunner(catalog, adapter, handlers, work.RunnerOptions{
    WorkerID: "notification-api",
})
go func() { _ = runner.Run(shutdownContext) }()
```

Errors are retryable by default. Backoff is bounded exponential with
deterministic jitter. `Permanent` disables retry; `RetryAfter` overrides the
next delay. Delivery is at least once, so external provider calls and other
side effects still need consumer-owned idempotency.

Pause or cancel invalidates a running lease. A stale handler cannot commit
progress or a result. Crashed or timed-out leases are reclaimed and recorded as
`lease_expired`.

## PostgreSQL

Apply generated migrations before constructing the Adapter:

```sh
go run ./work/postgres/cmd/workschema \
  -dir ./manifest/sql/migrations \
  -name 0015_work_v1
```

```go
adapter, err := workpostgres.New(ctx, catalog, workpostgres.Options{
    DB: consumerDB, InstanceKey: "notification:default",
})
```

The generator embeds a digest and refuses to overwrite a drifted migration.
PostgreSQL claim uses `FOR UPDATE SKIP LOCKED`; rows remain durable truth.
Polling is sufficient for correctness and requires no Redis, broker or remote
scheduler.

## Operations

`Module` supports:

- job get/list and counts by state;
- pause, resume and cancel;
- terminal replay as a new linked Job;
- immutable Attempt history;
- explicit terminal retention pruning.

`failed` is the durable dead-job state. There is no separate hidden dead-letter
store. Operators can inspect the last error and Attempts, then replay without
erasing the original history.

## Verification

Every Adapter should run `worktest.Run`. Memory and PostgreSQL share the same
suite for concurrent idempotency, claims, leases, recovery, progress, terminal
history, replay, pruning and recurring schedule exactness.

```sh
go test -race ./work/...
go vet ./work/...
```
