# Discovery wire contracts

`discovery.v1` is produced by the ordinary-Go Discovery module and consumed by
framework Adapters. The Go implementation is the protocol semantics oracle;
these schemas and fixtures govern wire compatibility.

- Consumers must reject an unknown `contractVersion`.
- Additive optional fields are compatible within `discovery.v1`.
- Canonical, Open Graph URL and sitemap location must agree.
- A projection containing `noindex` must not contain a sitemap entry.
- Structured data is JSON data, not a caller-supplied script string.

Fixtures under `fixtures/valid` must be accepted. Fixtures under
`fixtures/invalid` are structurally or semantically invalid and must fail
closed.
