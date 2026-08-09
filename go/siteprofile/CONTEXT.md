# Site Profile context

## Ownership

- One deployed site instance owns one Profile in its own database.
- Identity, branding, announcement, support contacts, footer links, social
  links, legal links, and compliance records belong to the Profile.
- Product runtime settings and homepage content remain with the product.

## Invariants

- A valid persisted state has one canonical normalized document and digest.
- Revision starts at one and changes only when the normalized document changes.
- Every write is conditional; there is no unconditional overwrite path.
- Stable IDs identify repeated contacts, links, groups, and compliance records. They are canonical UUIDv7 values assigned by this Module through Foundation Identifier; client draft keys and semantic kinds never become persisted IDs.
- Links reject unsafe schemes; request Host never participates in validation or
  public URL construction.
- Scheduled announcements are projected against an explicit clock and reveal
  their next transition time.
- Schema and validation derive from the same compiled Definition.

## Transaction rule

The PostgreSQL Store can bind a caller-owned `*sql.Tx`. The caller owns commit
and rollback. The Module does not keep an in-process data cache that could be
repopulated before that external transaction commits; revision-based ETag
revalidation provides the cache invalidation contract.
