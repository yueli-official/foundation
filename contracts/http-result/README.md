# HTTP Result contract

This contract standardizes JSON HTTP success shapes and product-owned public error catalogs without introducing a universal success envelope.

Success responses use the HTTP status and the endpoint DTO directly:

- resource reads return the resource DTO with `200`;
- bounded unpaginated collections return `{items}` with `200`;
- offset pages return `{items,page,size,total}` with `200`;
- cursor pages return `{items,nextCursor?}` with `200`;
- creation returns the resource DTO with `201`;
- accepted asynchronous work returns an operation DTO with `202`;
- successful operations without a body return `204`;
- binary bodies and redirects use dedicated adapters.

Failures use the sibling HTTP Problem v1 contract. OAuth/OIDC endpoints retain their protocol-defined error format.

`error-catalog.v1.schema.json` describes product-owned stable error semantics. `operations.v1.schema.json` describes only an operation's success status/body kind and public failure set; product DTO fields remain owned by OpenAPI and are referenced rather than duplicated.
