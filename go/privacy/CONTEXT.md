# Privacy Context

## Glossary

- **Data Subject**: a natural person, not an Identity or Authorization row.
- **SubjectRef**: an opaque, Owner-scoped locator used for one decision or
  workflow. Privacy does not create an alias graph.
- **Processing Purpose**: an exact revision of a declared reason, basis, data
  category set, notice and signal behavior.
- **Notice Revision**: immutable reference and content digest for
  consumer-owned notice text.
- **Consent Receipt**: affirmative evidence for exact consent-purpose and
  notice revisions.
- **Withdrawal**: append-only evidence ending future reliance on consent; it is
  not erasure.
- **Privacy Signal**: bounded normalized input such as GPC. Missing signal is
  not consent.
- **Retention Rule**: calendar eligibility for Owner review, never permission
  for Foundation to delete source rows.
- **Rights Request**: verified access, portability, rectification, erasure,
  restriction or objection aggregate.
- **Owner Task**: immutable, fingerprinted command to one data Owner.
- **Owner Receipt**: minimal, idempotent outcome evidence.
- **Anonymization**: irreversible loss of reasonable relation to a Data
  Subject. A reversible/keyed replacement remains pseudonymization.

## Invariants

- Policy and evidence truth is instance-local.
- Runtime callers bind a Purpose and cannot choose a basis per request.
- Historical evidence never flows to a changed Purpose or Notice revision.
- Consent is one basis; non-consent processing never receives synthetic
  consent.
- GPC affects only explicitly mapped Purpose revisions.
- Identity can coordinate but cannot decide another Owner's disposition.
- Cross-Owner handling is a durable saga, not a distributed transaction.
- Authentication-destroying Owners finalize only after all other Owners are
  terminal; this is a protocol gate, not a task-name convention.
- Only a valid Owner Receipt makes a task terminal.
- Retained/refused are honest terminal exceptions, not successful deletion.
- Work, Audit, Notification and Asset remain independent capabilities.

## Non-responsibilities

- legal advice, jurisdiction selection or lawful-basis selection;
- notice prose, Cookie Banner rendering or frontend preference UX;
- central customer profile, cross-site consent database or data lake;
- product record discovery, arbitrary SQL or provider deletion;
- source export payload storage;
- roles and authorization;
- Work execution, Audit retention, Notification delivery or backup expiry.
