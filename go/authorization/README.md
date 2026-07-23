# Authorization

`github.com/yueli-official/foundation/go/authorization` is the public,
ordinary-Go authorization module for independently deployed product instances.
Each consumer owns its catalog, PostgreSQL truth, administrators, roles and
policy. Identity proves a stable subject; it does not confer product roles.

The public API contains no SQL, Casbin tuple, HTTP framework or product DTO.
The current v1 schema and adapters are implemented and conformance-tested.

## Model

- **Access layers** (`visitor`, `authenticated`) are automatic and cannot be
  granted as roles. Anonymous public read and signed-in application
  capabilities belong here.
- **Roles** are allow-only capability bundles. Every consumer declares exactly
  one protected administrator role; business roles and their names are
  consumer-owned. Administrators can version built-in bindings and create
  scope-owned custom roles from catalog capabilities.
- **Scopes** form a consumer-declared tree. A grant applies to its scope and
  descendants. `ResourceScopeRegistry` lets trusted product lifecycle code
  idempotently register resources without granting users
  `authorization.manage`.
- **Constraints** are typed, code-owned rules that administrators cannot
  disable. Use them for invariants such as “every non-protected role can update
  only resources related as owner.”
- **Grants** retain source, validity, expiry and revocation provenance. Groups
  are flat User/Service sets; there is no role or group inheritance.
- **Applications**, **invitations** and **automatic rules** are distinct grant
  workflows. Disabling an automatic rule affects future reconciliation and
  does not revoke existing grants.
- **Policy revisions** stage role/access-layer bindings and automatic-rule
  switches, then validate, preview and atomically activate with optimistic
  revision checks.

Decisions default-deny and return a policy revision, stable reason and complete
source provenance. Query planning returns a closed AST (`all`, `none`,
`relation`, `scope_in`, `any`, `all_of`), never SQL.

## Consumer declaration

```go
catalog := authorization.MustCompile(authorization.Definition{
    Consumer: "docs",
    Version:  1,
    Capabilities: []authorization.CapabilityDefinition{{
        Key: "docs.document.update", Version: 1,
        Binding: authorization.BindingNormal,
        AllowedScopes: []authorization.ScopeType{"document"},
        EligibleSubjects: []authorization.SubjectKind{authorization.SubjectUser},
        QueryableRelation: "owner",
        Delegable: true,
    }},
    Scopes: authorization.ScopeSchema{Types: []authorization.ScopeTypeDefinition{
        {Key: "site", Root: true, Children: []authorization.ScopeType{"document"}},
        {Key: "document"},
    }},
    AccessLayers: []authorization.AccessLayerDefinition{
        {Key: authorization.AccessLayerVisitor},
        {
            Key: authorization.AccessLayerAuthenticated,
            Capabilities: []authorization.CapabilityKey{
                authorization.CapabilityApplicationCreate,
                authorization.CapabilityApplicationReadOwn,
                authorization.CapabilityApplicationWithdraw,
            },
        },
    },
    Roles: []authorization.RoleDefinition{
        {
            Key: "administrator", DisplayName: "管理员", Protected: true,
            Capabilities: []authorization.CapabilityKey{
                authorization.CapabilityManage,
                "docs.document.update",
            },
        },
        {
            Key: "author", DisplayName: "作者",
            Capabilities: []authorization.CapabilityKey{"docs.document.update"},
            Assignment: authorization.AssignmentPolicy{Sources: []authorization.GrantSource{
                authorization.GrantSourceApplication,
                authorization.GrantSourceInvitation,
                authorization.GrantSourceDirect,
                authorization.GrantSourceAutomatic,
            }},
        },
    },
    Constraints: []authorization.ConstraintDefinition{{
        Key: "docs.normal_role_owns_document", Version: 1,
        Mode: authorization.ConstraintSource,
        Capabilities: []authorization.CapabilityKey{"docs.document.update"},
        AllNormalRoles: true,
    }},
})
```

Attach every declared constraint and automatic predicate at construction. A
missing or unknown evaluator fails startup.

## Adapters

`Memory` is the reference adapter for tests and ephemeral consumers:

```go
module, err := authorization.NewMemory(catalog, authorization.MemoryOptions{
    RootScopeID: "docs",
    ProtectedSubjects: []authorization.SubjectRef{{
        Kind: authorization.SubjectUser, ID: bootstrapSubject,
    }},
    Constraints: constraintEvaluators,
    Predicates: predicateEvaluators,
})
```

`authorization/postgres` persists normalized domain truth in the consumer's own
database and uses an atomically replaced Casbin v3 projection only as an allow
candidate index. Constraints, validity, revocation, workflow state and
provenance remain domain-kernel responsibilities:

```go
db, _ := sql.Open("postgres", consumerDSN)
module, err := authorizationpostgres.New(ctx, catalog, authorizationpostgres.Options{
    DB: db,
    InstanceKey: "docs:default",
    Memory: authorization.MemoryOptions{
        RootScopeID: "docs",
        ProtectedSubjects: bootstrapSubjects, // only if the instance is absent
        Constraints: constraintEvaluators,
        Predicates: predicateEvaluators,
    },
})
```

The adapter fails closed on database/projection errors. Startup rebuilds the
in-process projection from domain tables; `RebuildProjection` also recreates
the inspectable database projection. Deleting the projection does not remove
authorization truth.

Generate migrations into the consumer repository; applications do not
auto-migrate:

```sh
go run ./authorization/postgres/cmd/authzschema \
  -dir ./manifest/sql/migrations \
  -name 0007_authorization_v1

go run ./audit/cmd/auditschema \
  -dir ./manifest/sql/migrations \
  -name 0008_audit_v1
```

Generated files contain a canonical digest. Re-running refuses to overwrite
drifted content. Apply each consumer migration with that product's normal
deployment path. The PostgreSQL Adapter stores management and decision audit
truth in the shared Audit Journal and requires both schemas. On first start
after the cutover it idempotently imports rows from the legacy
`authorization_audit_events` and `authorization_decision_events` tables when
they still exist; it never writes those legacy tables again.

## Consumer integration rules

1. Configure subjects only for first-instance administrator bootstrap. After
   creation, the local database is the sole authority.
2. Register existing and new product scopes through `ResourceScopeRegistry`;
   never let resource authors call `ScopeManager`.
3. Build `ResourceFacts` from product-owned relations, then call `Decide`.
4. Translate `QueryConstraint` through an allowlisted field mapping. Do not
   fetch all rows and treat UI filtering as authorization.
5. Bind actor identity from verified transport context, never a request body.
   Management methods authorize their actor inside the mutation.
6. Drive UI navigation and buttons from `EffectiveAccess`; the API remains the
   authority.

Docs demonstrates application/review, policy editing, owner constraints,
resource registration and capability-driven UI. Navigation is the second
conformance consumer and exercises a four-level scope tree, relation queries,
custom roles and delegated scope administration.

## Operations

- Run `authorizationtest.Run` for every adapter.
- Decision audit always records denies and high-risk allows; full-audit
  capabilities record every result. Management and decision evidence use the
  Foundation Audit Journal with the authorization state transaction.
- `postgres.RecoverProtectedAdministrator` is an offline, database-credential
  recovery command. It requires zero active protected administrators, a dry
  run and an exact confirmation string; it is not a runtime bypass.
- The current topology is one process instance per independent consumer
  database. There is no Redis, watcher, TTL cache, remote store or
  cross-instance synchronization protocol.
- Invitation plaintext tokens are returned only on issue/resend. Persistence
  retains only SHA-256 digests.

## Compatibility

Catalog keys are durable API. Change behavior by activating policy revisions;
increment the consumer definition version when code-owned capabilities, scope
topology or constraints change. The PostgreSQL adapter refuses catalog
version/digest mismatches and schema-version mismatches at startup. Upgrade
schema files through generated consumer migrations before deploying code that
requires them.

The module does not provide business role names, authentication, accounts,
resource storage, email delivery, product HTTP routes, UI, a central
authorization service or a platform super-administrator.
