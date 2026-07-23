# Site Profile

Site Profile is an instance-local ordinary-Go Module for the public identity and
footer material of one independently deployed website.

It owns normalization, validation, optimistic revision, strong ETag, scheduled
announcement projection, schema-driven management metadata, and persistence
semantics. It does not own product runtime settings, authorization, deployment
origins, theme tokens, asset bytes, or personal user profiles.

## Interface

```go
type Reader interface {
    Get(context.Context) (Snapshot, error)
}

type Replacer interface {
    Replace(context.Context, ReplaceCommand) (ReplaceResult, error)
}

type Describer interface {
    Schema() FormSchema
}
```

Revision zero is the bootstrap precondition. Every later Replace must carry the
exact current revision. Replacing with the same normalized document is a no-op
and retains the revision and ETag.

```go
definition := siteprofile.MustCompileDefinition(siteprofile.Definition{
    RequireTagline: true,
    RequireLogo: true,
})
module := siteprofile.NewMemory(definition, siteprofile.SystemClock{})

created, err := module.Replace(ctx, siteprofile.ReplaceCommand{
    ExpectedRevision: 0,
    Profile: profile,
})
```

## PostgreSQL

The Adapter persists one profile in each consumer database and never
auto-migrates:

```go
_, err := siteprofile.WritePostgresMigration(
    "manifest/sql/migrations",
    "0009_site_profile_v1",
    siteprofile.CurrentPostgresSchemaVersion,
    siteprofile.DefaultPostgresPrefix,
)
```

For atomic product orchestration, bind the Store to a caller-owned transaction:

```go
store, _ := siteprofile.NewPostgresStore(db, siteprofile.DefaultPostgresPrefix)
tx, _ := db.BeginTx(ctx, nil)
bound, _ := store.Bind(tx)
profiles := siteprofile.MustNew(bound, definition, clock)

// Update product runtime settings and the profile through the same tx.
_, err := profiles.Replace(ctx, command)
// The Module never commits or rolls back tx.
```

## HTTP

`httpadapter.Handler` provides separate public, admin, and schema handlers.
Consumers own routing and authorization.

- public reads use a strong ETag and `Cache-Control: public, no-cache`;
- admin reads use `private, no-store`;
- updates require `If-Match`, or `If-None-Match: *` for bootstrap;
- stale preconditions map to 412, missing preconditions to 428, validation to
  422, and corrupt state fails closed.

## Archive

`Export`, `VerifyArchive`, and `Restore` preserve the typed Profile with format,
schema, revision, document and archive digests. Restore always re-enters the
normal conditional Replace path; an archive cannot bypass current validation or
optimistic concurrency.

## Management Adapter

`@yueli/site-profile` validates FormSchema and Snapshot contracts and provides
headless field lookup, safe dotted-path access, dirty/reset state and
`If-Match` replacement requests. Its optional Vue export wraps the same editor;
products retain layout and authorization.

## Asset references

A visual is either an icon token or an opaque stable Asset ID. Site Profile
does not call the Asset service and never persists a URL derived from request
Host. Consumers verify an Asset before replacing the profile and maintain the
remote Asset reference through their reliable work path.

## Verification

```sh
go test -race ./siteprofile/...
go vet ./siteprofile/...
```

Real PostgreSQL conformance:

```sh
SITE_PROFILE_PG_DSN='postgres://user:pass@host:5432/postgres?sslmode=disable' \
go test -race ./siteprofile/... -count=1
```
