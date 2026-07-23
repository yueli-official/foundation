# Search

`github.com/yueli-official/foundation/go/search` is a public ordinary-Go Module
for instance-local content search. It owns a declared Search Document contract,
revision-safe projection, query normalization, filters, facets, structured
highlighting, keyset cursors and crash-resumable Rebuild Generations.

It is not a shared search service and does not own content or authorization.
Every consumer stores its Index in its own database, rehydrates Hits from its
Source Documents and rechecks current visibility before returning them.

## Definition

Compile logical analyzers and filter vocabulary at startup:

```go
catalog := search.MustCompile(search.Definition{
    Consumer: "docs.public", Version: 1,
    Analyzers: []search.AnalyzerDefinition{
        {
            Key: "zh-natural-v1", QueryMode: search.QueryWeb,
            Required: []search.Capability{
                search.CapabilityFullText,
                search.CapabilityChineseSegmentation,
            },
            ProbeText: "中文搜索", ProbeLexemes: []string{"中文", "搜索"},
        },
        {Key: "en-natural-v1", QueryMode: search.QueryWeb},
    },
    Filters: []search.FilterDefinition{
        {Name: "collection", Facetable: true},
        {Name: "locale"},
        {Name: "version"},
    },
})
```

The compiled Definition is an immutable compatibility contract. PostgreSQL
stores its version and digest and fails closed on silent drift. A breaking
Definition change uses a new, explicitly versioned Instance Key; backfill and
verify that instance before switching the consumer. Rebuild Generations repair
or refresh documents inside one immutable Definition and are not a hidden
schema-upgrade mechanism.

## Projection

`ProjectionRevision` must increase for every searchable change. Lower revisions
are stale, equal revisions with equal content are replays, and equal revisions
with different content conflict. Remove writes a tombstone so a delayed Upsert
cannot resurrect content.

```go
document := search.SourceDocument{
    Key: search.DocumentKey{Kind: "document", ID: "doc-1"},
    Revision: 42, Analyzer: "zh-natural-v1",
    Title: "权限模型", Summary: "管理员与作者能力",
    Body: body, SortAt: publishedAt,
    Filters: search.FieldValues{
        "collection": search.Keyword("guide"),
        "locale": search.Keyword("zh-CN"),
    },
    Visibility: search.VisibilityReference{
        ResourceType: "docs.document", ResourceID: "doc-1",
    },
}
_, err := module.Apply(ctx, search.Batch{
    ID: "docs.doc-1.42",
    Changes: []search.Change{search.Upsert(document)},
})
```

PostgreSQL supports caller-owned transactions:

```go
projector, err := adapter.Bind(tx)
if err == nil {
    _, err = projector.Apply(ctx, batch)
}
```

The caller owns commit and rollback. A Batch is validated and preflighted before
any projection write, and its durable receipt is stored in the same transaction.

## Query and visibility

```go
page, err := module.Search(ctx, search.Query{
    Text: "权限模型", Analyzer: "zh-natural-v1",
    Filters: []search.Filter{search.Equal("collection", "guide")},
    Facets: []search.FacetRequest{{Name: "collection", Limit: 20}},
    Page: search.PageRequest{Size: 20, Cursor: after},
    Highlight: search.HighlightRequest{
        Fields: []search.TextField{search.TextTitle, search.TextSummary},
    },
})
```

Empty text returns an empty Page; product browse endpoints remain product
responsibility. Relevance order is rank descending, SortAt descending and
DocumentKey ascending. Newest order uses SortAt and DocumentKey. Cursors bind
the Definition, normalized Query, Adapter, Generation and complete sort tuple.
They provide deterministic live keyset pagination, not a long-lived database
snapshot.

Score is comparable only inside one query, Adapter and Generation. Highlight
contains plain-text Segments with a `Matched` flag, never trusted HTML.

The consumer must batch-load each Hit's Source Document and confirm:

1. it still exists and is currently searchable;
2. the Projection Revision is still current;
3. the current subject may read the Visibility Reference.

Facet and Total describe Index candidates, not personalized post-authorization
counts.

## PostgreSQL analyzer capabilities

The first production Adapter uses native PostgreSQL FTS. Logical Analyzer Keys
are bound without leaking `regconfig` into core types:

```go
adapter, err := search.NewPostgres(ctx, catalog, search.PostgresOptions{
    DB: db, InstanceKey: "docs.default",
    AnalyzerBindings: map[search.AnalyzerKey]string{
        "zh-natural-v1": "chinese_zh",
        "en-natural-v1": "english",
    },
})
```

Construction verifies every explicit text-search configuration and optional
lexeme probe. Required capabilities fail closed; there is no silent `ILIKE`
fallback. `zhparser` is a deployment-provided PostgreSQL capability, not a core
dependency. PGroonga or a remote engine would be separate future Adapters.

## Rebuild

Rebuild uses an isolated building Generation:

1. `Start` creates it idempotently by request ID.
2. normal `Apply` dual-writes active and building Generations;
3. a Work handler resumes from `Status.Checkpoint` and sends bounded `Stage` batches;
4. `Finish` validates checkpoint and document count, then atomically activates;
5. old Generations remain available for existing cursors until an explicit
   `Prune` after the cursor TTL;
6. `Abandon` can remove only the current building Generation.

Search does not start workers or depend on Foundation Work. Work supplies
scheduling, leases and retries; the consumer owns Source Document enumeration.

## PostgreSQL schema

Generate a versioned migration:

```sh
go run ./search/cmd/searchschema \
  -dir ./manifest/sql/migrations \
  -name 0010_search_v1
```

The generator refuses to overwrite a drifted migration.

## Verification

Every Adapter should run `searchtest.Run`. Memory is the behavioral reference
for revision, tombstone, filters, facets, cursor binding, structured Highlight
and generation lifecycle; analyzer-specific linguistic scores are not required
to be numerically identical.

```sh
go test -race ./search/...
go vet ./search/...
```

Set `SEARCH_POSTGRES_DSN` to run real PostgreSQL conformance and transaction tests.
