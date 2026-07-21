# Platform migration ledger

Migration uses copy, independent validation, consumer cutover and only then source deletion. `platform` remains the production source until each consumer is switched to a versioned Foundation artifact.

| Platform evidence                | Foundation destination        | Status                         | Notes                                                                                                                  |
| -------------------------------- | ----------------------------- | ------------------------------ | ---------------------------------------------------------------------------------------------------------------------- |
| `contracts/http-problem`         | `contracts/http-problem`      | copied and validated           | Canonical cross-runtime schema and fixtures.                                                                           |
| `packages/js/http-runtime`       | `js/packages/http-runtime`    | copied and validated           | Rename review remains open before prerelease.                                                                          |
| `packages/js/nuxt-runtime`       | `js/packages/nuxt-runtime`    | copied and validated           | Explicit server/client Adapter boundary retained.                                                                      |
| `packages/js/yueli-ui`           | `js/packages/ui`              | migrated and deepened          | Added the experimental Nuxt module and public `CollectionFrame`; not a directory-for-directory copy.                   |
| `apps/http-runtime-conformance`  | `js/conformance/http-runtime` | copied and validated           | Production build and Playwright pass independently.                                                                    |
| `apps/ui-foundation-conformance` | `js/conformance/ui`           | migrated and validated         | Now consumes public `YCollectionFrame`; no workspace source-scanning escape hatch.                                     |
| `apps/ui-preview`                | `js/apps/ui-lab`              | curated migration started      | Only promoted, product-neutral Patterns enter the Lab. Legacy prototypes and platform-bound galleries remain evidence. |
| `packages/go/gokit`              | `go/*`                        | review-first; no source copied | See `go-batch-c.md`; first slice is Problem core plus GoFrame adapter.                                                 |

## Preview disposition

The old Preview imports `@platform/manage`, `@platform/ui` and `@platform/asset`, and includes company management-shell assumptions. Copying it wholesale would make the public repository depend on the platform it is meant to replace.

The Foundation UI Lab therefore consumes only public `@yueli/*` exports. Collection is present now. Dashboard, Settings, Feedback, BackToTop, media and user-menu examples enter only after their Interface, caller-owned i18n, tarball styling and real-consumer gates pass. Throwaway visual variants, fixture registries and debug screenshots are not migrated.

No platform implementation has been deleted yet. Deletion is authorized only after platform package references point at a versioned Foundation release or an explicitly temporary development override and all affected product gates pass.
