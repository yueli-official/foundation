# Classification Admin API

`github.com/yueli-official/foundation/go/classification/adminapi` is the
framework-neutral HTTP/JSON projection for administering Foundation
Classification. It is a wire contract, not an HTTP server: products still own
routes, authentication, persistence, transactions and failure mapping.

The canonical resources are `Category`, `Facet`, `FacetValue` and flat `Tag`.
Managed resources use `id`, `slug`, `name`, `status`, `revision`, optional
`parentId`, `editorialPosition` and `replacementId`; `FacetValue` always carries
its owner `facetId`. `description` is shared admin metadata and does not alter
Classification rules.

Facet assignment policy is also canonical: `minValues`, `maxValues`,
`leafOnly` and `maxDepth`. Product binding state is outside this package.

Products may extend a canonical DTO with typed product fields. For example a
category response may add an icon, image reference or contextual usage count.
Do not add those fields to the Foundation DTO and do not introduce an untyped
`extensions` bag merely to normalize one product.

For a selected Classification scope, the recommended resource surface is:

```text
GET    /admin/classification
GET    /admin/classification/categories
PUT    /admin/classification/categories/{id}
DELETE /admin/classification/categories/{id}
GET    /admin/classification/facets
PUT    /admin/classification/facets/{id}
GET    /admin/classification/facets/{facetId}/values
PUT    /admin/classification/facets/{facetId}/values/{id}
GET    /admin/classification/tags
PUT    /admin/classification/tags/{id}
```

A product with several Classification scopes may mount the same relative
surface once per scope. The scope selector and API root remain product routing
concerns rather than fields in the canonical DTO.

Small collection reads return `{ "items": [...] }`. Successful create/update
returns the resulting resource DTO. Successful deletion has no response body.
Optimistic concurrency uses the resource `revision`; a create starts from
revision `0` and an update supplies the revision it read.
