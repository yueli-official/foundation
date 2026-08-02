# Classification

`github.com/yueli-official/foundation/go/classification` is a public,
framework-neutral rules module for Category, Facet, Facet Value and Tag
classification. It compiles a consumer-owned Snapshot into an immutable
Catalog and evaluates classification, discovery and governance without owning a
database, transaction, HTTP API or background process.

Each product instance owns its Catalog rows, assignments, tag lookup registry,
contextual counts, governance transactions and public presentation. The module
owns the rules those adapters must obey.

## Domain

- Category is the primary browse tree. A subject may have multiple direct
  assignments and, when policy requires it, one separately stored Primary
  Category selected from those assignments.
- Facet is a controlled filter axis. Facet Values may form a tree only inside
  their owning Facet.
- Tag is always flat. Canonical names, aliases and replacements resolve through
  a consumer-owned lookup registry.
- Policy Profile narrows the shared model for a resource kind: cardinality,
  Primary Category, leaf/depth restrictions, unknown Tag admission and
  candidate ordering.

Category and Facet Value trees store only direct parents. A parent filter
expands to effective-active descendants. Filter groups use OR within a group
and AND between groups.

## Compile

Load one revision-consistent Snapshot from product storage and compile it:

```go
result := classification.Compile(classification.Snapshot{
	CatalogID: "blog",
	Revision:  42,
	Categories: []classification.Category{{
		ID: "engineering", Slug: "engineering", Name: "Engineering",
		Status: classification.StatusActive,
	}},
	Policies: []classification.PolicyProfile{{
		Key: "blog.post.default", SchemaVersion: 1, PolicyRevision: 3,
		Category: classification.CategoryPolicy{MaxAssignments: 8},
		Tags: classification.TagAdmissionPolicy{
			Unknown: classification.UnknownTagCreate,
			MaxAssignments: 20,
		},
	}},
})
if result.Outcome != classification.OutcomeAccepted {
	// Refuse startup or the catalog refresh and surface result.Diagnostics.
}
catalog := result.Catalog
```

`Compile` validates identity uniqueness, parent scope and cycles, lifecycle and
replacement invariants, Policy ranges and referenced Facets. The returned
Catalog does not expose mutable internal state.

## Classify

`Classify` validates a requested assignment set. Dynamic Tag resolution uses a
two-stage contract:

```go
preparation := catalog.Classify(classification.ClassifyRequest{
	PolicyKey:   "blog.post.default",
	CategoryIDs: []string{"engineering"},
	Tags:        []string{"Go", "PostgreSQL"},
})
request := preparation.FactRequest()

// The consumer batch-resolves request.TagLookups in its own registry.
facts := classification.ClassifyFacts{
	CatalogRevision: request.CatalogRevision,
	RequestToken:    request.RequestToken,
	FreshnessToken:  registryRevision,
	TagMatches:      matches,
}
decision := preparation.Complete(facts)
```

On acceptance, persist only `decision.Assignments` and explicitly process any
Tag Creations or Proposals in the same consumer-owned workflow. Never persist
the caller's unvalidated input.

## Discover

`Discover` resolves ID or slug references, replacements and descendants into a
canonical Filter Plan. Candidate projections are optional:

- `available` is intended for public discovery and uses contextual counts;
- `active` returns all effective-active options for edit forms;
- no projection returns only the normalized Filter Plan.

Contextual counts exclude the candidate's own group while retaining all other
groups. The consumer executes the typed count requests against its own visible
resource scope and completes the preparation with a freshness token.

## Govern

`Govern` plans lifecycle changes, reparent, merge and explicit deletion.
Dynamic child/assignment/replacement impacts are supplied as Facts. The
resulting Governance Plan is not a write API: the consumer must recheck catalog
revision, request token and impact freshness inside its transaction before
executing every step.

Merge never hides child handling. Delete never relies on an accidental cascade.
Facet merge is deliberately unsupported until a complete cross-value mapping
contract exists.

## Diagnostics and failures

Expected domain rejection uses stable typed Diagnostics and an Outcome. Storage,
context, network and transaction failures remain ordinary Go errors in the
consumer adapter; the core cannot produce them.

Stale, mismatched or incomplete Facts fail closed. Cache invalidation is a
consumer concern; revision comparison remains the source of truth.

## Persistence boundary

A production consumer normally stores:

- one revisioned Catalog and versioned Policy Profiles;
- separate Category, Facet, Facet Value and Tag tables with real foreign keys;
- separate assignment tables per identity kind;
- a Primary Category companion only when required;
- a normalized Tag lookup registry;
- governance evidence/outbox appropriate to that product.

Do not create a shared classification database or a generic polymorphic
assignment table. Products may use different PostgreSQL instances or entirely
different storage while sharing the same rules.

## Verification

The package tests exercise structural compilation, classification, discovery,
contextual candidates, fact freshness and governance plans. Consumer modules
can additionally run the representative policy contract:

```go
func TestClassificationPolicies(t *testing.T) {
	classificationtest.Run(t)
}
```

```sh
go test -race ./classification/...
go vet ./classification/...
```
