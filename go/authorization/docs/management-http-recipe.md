# Management HTTP recipe

Authorization deliberately does not own an HTTP framework. A consumer
controller authenticates the request, maps the verified principal to
`SubjectRef`, calls one command/query Interface, and maps the typed result.
The actor must never be accepted from JSON.

## Suggested resources

| HTTP resource | Public Interface | Notes |
| --- | --- | --- |
| `GET /authorization/effective-access` | `AccessReader` | Drive UI only; API checks remain authoritative. |
| `GET /authorization/scopes` | `ScopeReader` | Administrator view; resource lifecycle uses `ResourceScopeRegistry`. |
| `GET/POST/PATCH /authorization/roles` | `RoleReader`, `RoleManager` | Role writes target a draft revision. |
| `GET/POST/DELETE /authorization/grants` | `GrantReader`, `GrantManager` | Use explicit source, validity and expiry. |
| `GET/POST /authorization/groups` | `GroupReader`, `GroupManager` | Groups are flat; reject group members of kind Group. |
| `GET/POST /authorization/applications` | `WorkflowReader`, `WorkflowManager` | Separate own and managed list routes to avoid enumeration. |
| `GET/POST /authorization/invitations` | `WorkflowReader`, `WorkflowManager` | Return plaintext token only from issue/resend responses. |
| `GET/POST /authorization/policies` | `PolicyReader`, `PolicyManager` | Draft → edit → validate/preview → activate. |
| `GET /authorization/audit` | `AuditReader`, `DecisionAuditReader` | Bound page sizes and filters. |

An application endpoint should use `ListRequestableRoles` instead of exposing
all roles. Own application queries set both `Actor` and `Subject` to the
verified caller. Managed queries omit `Subject`; the module then requires
management access.

## Error mapping

| `ErrorKind` | HTTP status | Public behavior |
| --- | --- | --- |
| `invalid_input` | 400 | Return field-safe validation information. |
| `denied` | 403 | Use the same shape for inaccessible managed objects. |
| `not_found` | 404 | Use only after enumeration policy permits it. |
| `conflict`, `invariant_violation` | 409 | Includes stale revision, terminal state and last administrator. |
| `expired` | 410 | Expired invitation/application token. |
| `unavailable` | 503 | Fail closed; do not convert to a deny or retry a mutation blindly. |

Never return Adapter diagnostics, token digests, Identity facts or database
errors. Use an external request/trace ID in transport logs and include returned
Decision IDs when a product action is denied.

Attach trusted trace metadata with `authorization.WithRequestMetadata`; audit
queries can filter the persisted correlation ID. Application creation accepts
an explicit `IdempotencyKey`: replaying the same subject and command returns the
original application, while reusing the key for different input conflicts.
HTTP consumers should map this from `Idempotency-Key`, not from the JSON body.

## OpenAPI fragment

```yaml
paths:
  /api/v1/authorization/applications:
    post:
      summary: Apply for a requestable role
      security: [{ bearerAuth: [] }]
      parameters:
        - in: header
          name: Idempotency-Key
          schema: { type: string, maxLength: 200 }
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [role]
              properties:
                role: { type: string }
                reason: { type: string, maxLength: 2000 }
      responses:
        "200": { description: Pending application }
        "409": { description: Pending application already exists }
  /api/v1/authorization/manage/applications/{id}/review:
    post:
      summary: Review an application
      security: [{ bearerAuth: [] }]
      parameters:
        - in: path
          name: id
          required: true
          schema: { type: string }
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [decision]
              properties:
                decision: { type: string, enum: [approve, reject] }
                reason: { type: string, maxLength: 2000 }
      responses:
        "200": { description: Terminal application and optional grant ID }
        "403": { description: Insufficient scoped management authority }
        "409": { description: Application is already terminal }
```

Docs contains a concrete GoFrame mapping and generated OpenAPI routes. Its
controllers contain no policy logic: ownership remains a typed Constraint and
role/policy state remains in the public module.
