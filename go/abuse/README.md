# Abuse

`github.com/yueli-official/foundation/go/abuse` is a public ordinary-Go Module
for instance-local admission controls. Consumers declare stable Actions and
their independent Network, Actor or Target meters; the Module atomically
returns `allow`, `challenge` or `reject`.

It is not a shared risk service, HTTP middleware, account lock, bot score,
Authorization engine or content moderator. Each consumer stores truth in its
own PostgreSQL database.

## Define and bind Actions

Endpoint code never sends Policy IDs, costs or algorithms. Compile those once
and bind the Action during startup:

```go
catalog := abuse.MustCompile(abuse.Definition{
    Version:  1,
    Consumer: "identity",
    Actions: []abuse.ActionDefinition{{
        Key: "identity.password_login",
        Required: abuse.SignalRequirements{
            Network: abuse.Required,
            Target:  abuse.Required,
        },
        Meters: []abuse.MeterDefinition{
            {
                ID: "login.network",
                Slot: abuse.SlotNetwork,
                Algorithm: abuse.TokenBucket(30, 30, 5*time.Minute),
            },
            {
                ID: "login.target_failures",
                Slot: abuse.SlotTarget,
                Mode: abuse.MeterOutcome,
                Algorithm: abuse.SlidingWindow(20, 30*time.Minute),
                ChallengeAt: 6,
                ChargeOn: []abuse.OutcomeKey{"credentials_rejected"},
                ResetOn: []abuse.OutcomeKey{"authenticated"},
            },
        },
        Resolution: &abuse.ResolutionDefinition{
            Outcomes: []abuse.OutcomeKey{
                "authenticated",
                "credentials_rejected",
            },
            DefaultOutcome: "credentials_rejected",
            PendingTTL: time.Minute,
        },
        Challenge: &abuse.ChallengeDefinition{
            Kind: "turnstile",
            ExpectedAction: "identity-login",
            AllowedHosts: []string{"account.example.test"},
        },
    }},
})

login, err := module.Action("identity.password_login")
```

Thresholds are consumer policy. Foundation deliberately ships no hidden
universal login, signup, comment or publishing limit.

## Admit and resolve

The consumer performs trusted-proxy parsing and canonicalizes the target before
calling Abuse:

```go
admission, err := login.Admit(ctx, abuse.Input{
    ID: requestID,
    Signals: abuse.Signals{
        Network: clientPrefix,
        Target: canonicalEmail,
    },
    Proof: proof,
})
if err != nil {
    // Operational failure: do not continue.
}
if admission.Disposition != abuse.DispositionAllow {
    // The HTTP adapter chooses a generic challenge or 429 response.
}

valid := verifyPassword(...)
outcome := abuse.OutcomeKey("credentials_rejected")
if valid {
    outcome = "authenticated"
}
if err := login.Resolve(ctx, admission.Receipt, outcome); err != nil {
    // Resolve before creating the session; do not silently ignore failure.
}
```

Outcome meters create a pending event before password work. Pending events
already consume capacity, so a crash or omitted failure report cannot evade
the budget. A success removes its own pending event and only the already
committed Target failures selected by the Definition. It never clears another
concurrent pending attempt or a Network admission budget.

Actions without outcome meters only call `Admit`.

`AttemptID` is idempotency, not a reusable permission. An identical retry
returns `Admission.Replay=true`; a non-idempotent consumer must not execute its
business mutation again on such a replay. Use a server-generated request ID,
or couple it to the product's own idempotency key and result. A challenged
Attempt may continue with the same ID and a proof.

## Privacy boundary

Signals are transient. The Module HMACs each canonical value with the instance
secret, key version, Policy and slot before persistence. PostgreSQL does not
store raw IP addresses, account IDs, email addresses, User-Agent strings or
proof tokens. Do not put those values in Attempt IDs.

Network is a `netip.Prefix`, not a raw header string. The consumer must select
trusted proxy headers and the intended IPv4/IPv6 prefix before calling Abuse.
User-Agent and device fingerprints are intentionally absent from the base API.

## PostgreSQL

```go
module, err := abuse.NewPostgres(ctx, catalog, abuse.PostgresOptions{
    DB:          db,
    InstanceKey: "identity:account",
    Verifiers: map[abuse.ChallengeKind]abuse.ChallengeVerifier{
        "turnstile": turnstileVerifier,
    },
})
```

Every `Admit` and `Resolve` owns a short database transaction. There is no
caller-transaction binding: a downstream validation or business rollback must
not refund an attempted operation. Multiple meter rows are locked in canonical
order. Provider verification happens outside those transactions and is
followed by a complete budget re-evaluation.

Generate immutable consumer migrations:

```sh
go run ./abuse/cmd/abuseschema \
  -dir ./manifest/sql/migrations \
  -name 0017_abuse_v1
```

A monotonic Definition upgrade may change capacity/refill/window/retention when
the Policy Revision also increases. Existing consumption is preserved and
clamped. Changing algorithm kind, Signal slot or admission/outcome mode
requires a new Policy ID. Version rollback, same-version digest drift and
same-revision Policy drift fail startup.

## Challenge adapters

`ChallengeVerifier` is the only remote Port. The Turnstile Adapter performs
server-side Siteverify, reuses one verification UUID across network retries,
checks expected action and hostname, rejects missing/overlong tokens before
network I/O, and keeps invalid proof distinct from provider unavailability.

```go
verifier, err := turnstile.New(turnstile.Options{
    Secret: os.Getenv("TURNSTILE_SECRET"),
})
```

No verifier call is made until local budgets actually require a challenge.
Valid proof satisfies only a challenge tier; it cannot bypass a hard reject.

## Governance and retention

`module.Governor()` exposes Action inventory, exact-subject inspection, reset
and bounded prune. Governance receives the same transient Signals and derives
the same pseudonyms; administrators do not paste durable HMAC keys into normal
workflows. Keep authorization and audit of those commands in the consumer.

Run Prune on a local Work schedule and alert on failure. Delayed cleanup may
increase storage but does not change live decisions.

## Verification

Memory is the deterministic reference Adapter. PostgreSQL runs the same
`abusetest.Run` suite, plus exact concurrent-spend, definition upgrade and
durable privacy tests. Turnstile protocol tests cover retries, action/hostname
binding, size limits, invalid proofs and redaction.
