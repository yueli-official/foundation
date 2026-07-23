# Audit

`github.com/yueli-official/foundation/go/audit` is a public ordinary-Go module
for instance-local audit journals. Consumers define their own versioned Actions
and Evidence allowlists; the module owns validation, idempotency, strict
sequence assignment, tamper-evident digests, bounded reads, streaming export,
legal hold, retention and committed-event mirrors.

It is not a central audit service, a general event store or a replacement for
PostgreSQL/operating-system administrator auditing.

## Compile the consumer contract

Actions, target types, Evidence fields and Retention Classes are consumer
vocabulary. Evidence has a closed type set and cannot contain arbitrary JSON or
free-form text. Stable references use `Reference`/`References`; enumerated
codes use `Code`/`Codes`.

```go
catalog := audit.MustCompile(audit.Definition{
    Version:  1,
    Consumer: "docs.main",
    Retention: []audit.RetentionDefinition{{
        Class: "retention.management",
        MinimumAge: 365 * 24 * time.Hour,
        ArchiveBefore: true,
    }},
    Actions: []audit.ActionDefinition{{
        Action: audit.Action{Name: "docs.document.published", Version: 1},
        Category: audit.CategoryAdministration,
        TargetTypes: []string{"docs.document"},
        Commit: audit.CommitAtomicRequired,
        Retention: "retention.management",
        Evidence: []audit.FieldDefinition{
            {Key: "document.revision", Kind: audit.EvidenceCount, Required: true},
            {Key: "document.digest", Kind: audit.EvidenceDigest, Required: true},
        },
    }},
})
```

An existing Action name and version is immutable. Add a new Action version when
its schema changes. The PostgreSQL adapter rejects incompatible stored
definitions at startup.

Bind a typed consumer projector once:

```go
type publishEvidence struct {
    Revision uint64
    Digest   string
}

published := audit.MustBindAction(
    catalog,
    audit.Action{Name: "docs.document.published", Version: 1},
    func(value publishEvidence) []audit.EvidenceField {
        return []audit.EvidenceField{
            audit.Count("document.revision", value.Revision),
            audit.EvidenceDigestValue("document.digest", value.Digest),
        }
    },
)
```

Do not put passwords, tokens, cookies, sessions, request/response bodies,
connection strings, raw email/IP addresses or business payloads in Actor,
Target, Correlation or Evidence. Use stable references, reason codes, counts,
field names and precomputed digests.

## Write atomically with the product mutation

The durable adapter stores truth in the consumer's PostgreSQL database.
Management/configuration Actions default to `CommitAtomicRequired`:

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil { /* fail mutation */ }
defer tx.Rollback()

// Write product state through tx first.

appender, err := journal.Bind(tx)
if err != nil { /* fail mutation */ }
_, err = audit.Record(ctx, appender, published, audit.Attempt[publishEvidence]{
    ID: audit.DeriveEventID(commandID,
        audit.Action{Name: "docs.document.published", Version: 1}, 0),
    Actor: audit.Actor{Kind: audit.ActorUser, ID: subjectID},
    Target: audit.Target{Type: "docs.document", ID: documentID},
    Outcome: audit.Outcome{Kind: audit.OutcomeSucceeded},
    Correlation: audit.Correlation{
        RequestID: requestID,
        TraceID: traceID,
        SpanID: spanID,
        CommandID: commandID,
    },
    Evidence: publishEvidence{Revision: revision, Digest: digest},
})
if err != nil { /* rollback product mutation */ }
if err := tx.Commit(); err != nil { /* report mutation failure */ }
```

Append failure rolls the product mutation back. A stable EventID plus identical
prepared content replays the original Event; reusing it for different content
is an idempotency conflict. IDs of legally purged Events remain reserved.

`AppendIndependent` is available only for Actions explicitly compiled with
`CommitIndependentAllow`.

## Query, export and verify

Queries use descending Sequence keysets and opaque filter-bound cursors. The
limit defaults to 100 and is capped at 500; there is no offset or implicit
total-count query.

```go
page, err := journal.Query(ctx, audit.Query{
    Actor: &audit.Actor{Kind: audit.ActorUser, ID: subjectID},
    Actions: []audit.Action{{Name: "docs.document.published", Version: 1}},
})
```

`Export` streams versioned NDJSON in ascending Sequence order and returns a
content digest. Discard the destination if it returns an error.

`Verify` checks Event canonical digests, chain links, the ledger head and legal
retention gap receipts. This is tamper evidence, not proof against a database
owner or superuser. Higher-assurance deployments need an external HMAC key,
signed checkpoints or write-once archive outside the database.

## Legal hold and retention

Legal holds select explicit Event IDs and/or a bounded audit Query. Active
holds always override retention:

```go
_, err := journal.PlaceHold(ctx, audit.PlaceHoldCommand{
    ID: "case-2026-0042",
    Reason: "legal.request",
    Actor: audit.Actor{Kind: audit.ActorUser, ID: administratorID},
    Selection: audit.HoldSelection{
        Query: audit.Query{Target: &audit.Target{
            Type: "docs.document", ID: documentID,
        }},
    },
})
```

`RunRetention` removes at most `BatchLimit` eligible Events. If any selected
Retention Class requires archive, a consumer `ArchiveSink` must durably write
the supplied stream and return its exact count and content digest. The module
rechecks the journal head, holds and candidate set before purge, stores
idempotency tombstones and creates chain-gap receipts.

Archive lifecycle, encryption, access policy, geographic placement and object
deletion are deployment responsibilities.

## Committed mirrors

Set `EnableMirrorOutbox` on `PostgresOptions` to insert an outbox row in the
same transaction as every Event. `DispatchMirror` leases only committed rows,
publishes at least once and records retry state without changing Journal truth.
Use EventID for downstream deduplication.

`SlogMirror` emits bounded structured attributes. `audit/otelmirror` maps the
same committed Events to OpenTelemetry Log Records. Log filtering, sampling,
SDK/exporter failure or stdout availability never decides whether an Audit
Event exists.

## PostgreSQL migration

Applications do not auto-migrate. Generate immutable migrations into the
consumer repository:

```sh
go run ./audit/cmd/auditschema \
  -dir ./manifest/sql/migrations \
  -name 0018_audit_v1
```

The generator embeds the canonical schema digest and refuses to overwrite a
drifted file. Apply the up migration before `NewPostgres`; use the down
migration only after all consumer data has been deliberately retired.

## Error and failure policy

Use `audit.IsKind` with stable kinds such as `invalid_attempt`,
`rejected_evidence`, `transaction_required`, `idempotency_conflict`,
`integrity_mismatch`, `archive_required` and `hold_conflict`. Public error
messages never contain Evidence values, SQL or wrapped driver errors.

Required Audit append failures fail the product mutation. A committed mirror
failure does not roll back already committed Journal or product state; retry it
from the outbox and alert on `MirrorBacklog`.
