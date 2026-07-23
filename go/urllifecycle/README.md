# URL Lifecycle

URL Lifecycle is an instance-local ordinary-Go Module for canonical route
claims, aliases, permanent and temporary redirects, gone outcomes, atomic URL
transitions, and deterministic resolution.

It does not generate slugs, own product entities or publication state, authorize
administrators, derive trust from request headers, or publish sitemap/metadata.
Consumers derive complete public URL facts and commit them with product state in
the same PostgreSQL transaction.

## Interface

The mutation kernel has one semantic path:

```go
type Resolver interface {
    Resolve(context.Context, Lookup) (Resolution, error)
}

type Planner interface {
    Preview(context.Context, ChangeSet) (Plan, error)
}

type Transitioner interface {
    Apply(context.Context, ChangeSet, ApplyOptions) (Receipt, error)
}
```

`Claim`, `Rename`, `Merge`, `Rebase`, `Retire`, and
`SetTemporaryRedirect` are pure `ChangeSet` constructors. They do not bypass
the final-state planner.

`RouteKey` is stable while `LocalRef` is its current public identity:

```go
post := urllifecycle.RouteKey{
    Resource: urllifecycle.ResourceKey{Kind: "blog.post", ID: postID},
}

set := urllifecycle.Rename(
    urllifecycle.MutationMeta{
        CommandID: commandID,
        Actor:      urllifecycle.ActorRef{Kind: "user", ID: subject},
        Reason:     "post slug changed",
    },
    post,
    currentRevision,
    currentActiveRoute,
    urllifecycle.LocalRef{Path: "/posts/new-slug"},
    urllifecycle.DefaultPermanentRedirect(),
)
```

Docs uses `RouteKey.Variant` and registered identity-query dimensions for
locale/version representations that can share the same path:

```go
urllifecycle.RouteKey{
    Resource: urllifecycle.ResourceKey{Kind: "docs.doc", ID: doc.ID},
    Variant:  doc.Locale + ":" + doc.VersionKey,
}
```

## PostgreSQL

The Adapter persists truth in each consumer database. It never auto-migrates.
Generate an immutable consumer migration:

```go
_, err := postgres.WriteMigration(
    "manifest/sql/migrations",
    "0008_url_lifecycle_v1",
    postgres.CurrentSchemaVersion,
    postgres.DefaultPrefix,
)
```

Construct the Adapter at startup:

```go
catalog := urllifecycle.MustCompile(definition)
urls, err := postgres.New(ctx, catalog, postgres.Options{
    DB:          sqlDB,
    InstanceKey: "docs:default",
})
```

For atomic product updates, bind the caller-owned transaction:

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil { return err }
defer tx.Rollback()

bound, err := urls.Bind(tx)
if err != nil { return err }

if err := docs.UpdatePathTx(ctx, tx, delta); err != nil { return err }
if _, err := bound.Apply(ctx, changeSet, urllifecycle.ApplyOptions{}); err != nil {
    return err
}
return tx.Commit()
```

The bound Adapter never commits or rolls back. GoFrame consumers can pass
`gdb.TX.GetSqlTX()`.

## HTTP

`httpadapter.Middleware` depends only on `Resolver`.

- canonical and unknown normally continue to the product router;
- alias/redirect emits one validated `Location` with 301/302/307/308;
- gone emits 410;
- every terminal response has explicit `Cache-Control`;
- temporary freshness is bounded by overlay expiry;
- request Host and forwarded headers never influence Location.

303 is deliberately absent because it represents an action-result workflow,
not a URL rename or alias.

## Invariants

- one normalized `LocalRef` has at most one active base outcome;
- one `RouteKey` has at most one canonical claim;
- internal targets are stable RouteKeys, so public redirects remain one hop
  after repeated renames and merges;
- temporary redirects overlay and later reveal base state;
- former URLs are reusable only after explicit release;
- delete, archive, and merge never guess a retirement outcome;
- ChangeSets are unordered final-state declarations and commit all-or-nothing;
- command ID + normalized intent is replay-safe; ID reuse with different intent
  conflicts;
- unknown and gone remain distinct through export/restore;
- external targets are disabled unless their exact normalized origin is
  compiled into policy.

## Verification

```sh
go test -race ./urllifecycle/...
go vet ./urllifecycle/...
```

Real PostgreSQL conformance:

```sh
URL_LIFECYCLE_PG_DSN='postgres://postgres:postgres@host:5432/postgres?sslmode=disable' \
go test ./urllifecycle/... -count=1
```
