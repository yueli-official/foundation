# Privacy

`privacy` is an ordinary-Go, instance-local processing-policy and data-Owner
orchestration module. It provides:

- immutable, revision-bound consent, withdrawal and normalized signal evidence;
- bound Processing Purposes with fail-closed decisions;
- retention eligibility and Owner review state;
- verified Rights Requests with partial/overdue/exception-aware aggregation;
- an idempotent Owner command/receipt protocol;
- Memory and PostgreSQL implementations plus shared conformance tests;
- immutable PostgreSQL schema generation.

It is not a legal rules engine, Cookie Banner, customer profile, data lake,
arbitrary SQL deletion tool or replacement for Identity, Authorization, Work,
Audit, Notification, Asset or product-owned data.

## Model

Consumers compile typed definitions:

```go
catalog := privacy.MustCompile(privacy.Definition{
    Version:  privacy.DefinitionVersion,
    Consumer: "blog",
    SubjectKinds: []privacy.SubjectKindDefinition{
        {Key: "address", MaxRefBytes: 320},
    },
    DataCategories: []privacy.DataCategoryDefinition{
        {Key: "marketing_contact"},
    },
    Notices: []privacy.NoticeDefinition{{
        Ref:           privacy.NoticeRef{Key: "blog.newsletter", Revision: 3},
        ContentDigest: "sha256:...",
        Purposes: []privacy.PurposeRef{{
            Key: "blog.newsletter", Revision: 2,
        }},
        PublishedAt: publishedAt,
    }},
    Purposes: []privacy.PurposeDefinition{{
        Ref:        privacy.PurposeRef{Key: "blog.newsletter", Revision: 2},
        Basis:      privacy.BasisConsent,
        Categories: []privacy.DataCategoryKey{"marketing_contact"},
        Notices:    []privacy.NoticeRef{{Key: "blog.newsletter", Revision: 3}},
        SignalRules: []privacy.SignalRule{{
            Signal: "gpc", Effect: privacy.SignalDeny,
        }},
    }},
    ActivePurposes: []privacy.ActivePurpose{{
        Key: "blog.newsletter",
        Ref: privacy.PurposeRef{Key: "blog.newsletter", Revision: 2},
    }},
    Signals: []privacy.SignalDefinition{{
        Key: "gpc", MaxEvidenceAge: 24 * time.Hour,
    }},
})
```

`Key + Revision` is immutable. PostgreSQL reconciliation stores a digest for
every Notice, Purpose, retention rule and Owner definition and rejects a reused
revision with changed meaning.

The package ships closed lawful-basis and rights-operation vocabularies. It does
not choose which basis, purpose, notice, retention period or GPC mapping is
legally correct for a deployment.

## Runtime

Ordinary callers bind one active Purpose and cannot choose a basis at request
time:

```go
newsletter, err := runtime.Purpose("blog.newsletter")
if err != nil { return err }

decision, err := newsletter.Decide(ctx, privacy.DecisionInput{
    Subject: privacy.SingleSubject(addressRef),
    Signals: []privacy.ObservedSignal{{
        Signal: "gpc", AssertedAt: now,
    }},
})
if err != nil || !decision.Allows() {
    return nil // optional processing fails closed
}
```

Decision precedence is:

1. invalid/inactive revisions deny;
2. mapped request or persisted signals deny/restrict;
3. consent purposes require an exact affirmative receipt not superseded by a
   later withdrawal;
4. non-consent purposes follow their compiled basis without synthetic consent.

Absence of GPC is not consent. Runtime accepts only normalized signals, never raw
headers, IP addresses, user agents, Traffic markers or Abuse proofs.

## Consent and withdrawal

```go
receipt, err := runtime.Evidence().Consent(ctx, privacy.ConsentCommand{
    IdempotencyKey: "newsletter-confirm:" + confirmationID,
    Subject:        addressRef,
    Notice:         newsletterNoticeV3,
    Purposes:       []privacy.PurposeRef{newsletterPurposeV2},
    OccurredAt:     now,
    Channel:        "double_opt_in",
})
```

The exact Notice revision must include every exact consent-purpose revision.
Withdrawal appends a separate immutable event; it does not rewrite consent or
affect a non-consent purpose. Every mutating command has
same-key/same-fingerprint replay and same-key/different-fingerprint conflict.

## Retention

Retention rules compute calendar review eligibility:

```go
item, err := runtime.Retention().Track(ctx, privacy.RetentionCommand{
    IdempotencyKey: "comment:" + comment.ID + ":retention-v1",
    Record: privacy.RecordRef{
        Dataset: "blog.comments",
        Value:   comment.ID,
    },
    Rule:        commentNetworkRetention,
    TriggeredAt: comment.CreatedAt,
})
```

`Due` is a bounded cursor query, not a worker queue. Work schedules the query and
asks the Owner to review due records. Privacy never deletes product rows.
`retained` requires a reason and next review time unless an external hold
reference exists.

## Rights Requests

Identity verifies the requester and opens a Coordinator request. The Coordinator
freezes the active Owner registry and creates one stable task per capable Owner:

```go
view, err := coordinator.Open(ctx, privacy.OpenRightsRequest{
    IdempotencyKey: "privacy-request:" + requestID,
    Subject:        resolvedSubjectRefs,
    Operation:      privacy.RightErasure,
    Verification: privacy.VerificationEvidence{
        VerifiedAt:      now,
        Method:          "session_reauthentication",
        Assurance:       "high",
        VerificationRef: verificationID,
    },
    RequestedAt: now,
    Channel:     "self_service",
})
```

A Work handler calls bounded `Drive`. Privacy commits an in-flight task before
calling the Owner outside a database transaction. The Owner durably recognizes
the Task ID and command fingerprint; a lost response is recovered by replay.

```go
result, err := coordinator.Drive(ctx, privacy.DriveRightsRequest{
    Request: view.ID,
    Budget: privacy.DriveBudget{
        MaxOwnerAttempts: 4,
        MaxDuration:      20 * time.Second,
    },
})
```

Request completion is honest:

- `overdue` is independent of terminal phase and does not stop retries;
- `retained`, `refused` and `not_found` are distinct terminal outcomes;
- a request with exceptions never claims unqualified erasure success;
- Owner Receipts contain minimal category counts, reasons and expiring artifact
  references, not copied source data.

An Owner that destroys the subject's ability to authenticate can declare
`FinalizeAfterOwners`. The Coordinator will not claim that Owner until every
non-finalizing Owner task is terminal. Consumers must still provide a separate
status capability if the subject needs to read the final receipt after account
erasure.

## Owner Host

An Owner declares datasets and operations, then supplies record-level behavior:

```go
host, err := privacy.NewPostgresOwnerHost(ctx, catalog, options,
    privacy.OwnerExecutorFunc(func(
        ctx context.Context,
        instruction privacy.OwnerInstruction,
    ) (privacy.OwnerOutcome, error) {
        // Use instruction.Command.TaskID as the idempotency key for domain and
        // provider side effects. Return one result per requested dataset.
    }),
)
```

`OwnerHost.Handle` persists command acceptance before invoking the executor and
replays terminal receipts. The Owner—not Privacy—decides whether records are
exported, rectified, restricted, deleted, anonymized, retained or refused.

`httpadapter.NewClient` requires HTTPS for remote Owner endpoints. Loopback HTTP
is accepted for local tests. Deployments whose isolated service network does not
yet terminate TLS per workload may set `AllowInsecureHTTP` explicitly; never
enable that escape hatch for a public or untrusted network.

## PostgreSQL

Apply the immutable schema:

```sh
go run ./privacy/cmd/privacyschema \
  -dir /path/to/migrations \
  -name privacy_v1
```

Open adapters:

```go
runtime, err := privacy.NewPostgresRuntime(ctx, catalog, privacy.PostgresOptions{
    DB: db, InstanceKey: "blog",
})

coordinator, err := privacy.NewPostgresCoordinator(
    ctx, catalog,
    privacy.PostgresOptions{DB: db, InstanceKey: "identity"},
    ownerRouter,
)
```

`PostgresRuntime.Bind(tx)` joins evidence/retention changes to a caller-owned
transaction and never commits or rolls it back. Coordinator progress and Owner
calls use short module-owned transactions because no database transaction is
held over the network.

The PostgreSQL conformance suite uses `PRIVACY_PG_DSN`:

```sh
PRIVACY_PG_DSN='postgres://postgres:postgres@host:5432/postgres?sslmode=disable' \
  go test -race ./privacy/...
```

## Adjacent services

- Work transports retries and schedules; Privacy owns request/task meaning.
- Audit stores minimal governance evidence and legal holds.
- Notification delivers status or product messages; its mutable preference is
  not automatically a Consent Receipt.
- Asset or another Owner sink stores protected, expiring export payloads.
- Authorization protects operations but does not choose processing purposes.
- Traffic and Abuse keep their minimized/pseudonymized state; Privacy does not
  copy raw signals to make them subject-searchable.
