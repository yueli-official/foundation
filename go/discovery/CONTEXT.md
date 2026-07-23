# Discovery context

## Owns

- Trusted-origin URL compilation and cross-protocol identity consistency.
- Typed page projection for canonical, robots, Open Graph, X Card and JSON-LD.
- Streaming sitemap/index, RSS/Atom and robots rendering.
- Stable diagnostics, protocol limits and atomic publication semantics.

## Does not own

- Which product records are published, public or included in a feed.
- Product database queries, routing lifecycle or access control.
- Request Host interpretation, remote storage or cross-instance state.
- Nuxt rendering details.

## Stable seams

- An immutable `SiteProfile` snapshot enters `Compile`.
- Product queries implement `CursorSource`.
- Storage or transport implements `PublishTarget`.
- Nuxt consumes the versioned `PageProjection` wire contract.
