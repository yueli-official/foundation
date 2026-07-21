# HTTP Problem v1 contract

`http-problem.v1.schema.json` is the canonical cross-language profile for JSON API failures.
It extends RFC 9457 with required `code` and `traceId` members and optional structured `params`
and `violations`.

The schema owns the JSON shape. HTTP-aware adapters additionally enforce invariants that JSON
Schema cannot express:

- the HTTP status equals the body `status`;
- `content-type` is `application/problem+json`;
- an `x-trace-id` response header, when present, equals the body `traceId`;
- the error body stays within the configured byte limit.

`fixtures/valid` and `fixtures/invalid` are golden inputs for every TypeScript and Go codec. Add a
fixture before changing the contract. Product copy and translated messages do not belong here;
callers resolve `code + params` through their own i18n solution.
