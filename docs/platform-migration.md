# Platform migration ledger

Migration uses copy, independent validation, consumer cutover and only then source deletion. `platform` remains the production source until each consumer is switched to a versioned Foundation artifact.

| Platform evidence                | Foundation destination        | Status                   | Notes                                                                                                |
| -------------------------------- | ----------------------------- | ------------------------ | ---------------------------------------------------------------------------------------------------- |
| `contracts/http-problem`         | `contracts/http-problem`      | copied and validated     | Canonical cross-runtime schema and fixtures.                                                         |
| `packages/js/http-runtime`       | `js/packages/http-runtime`    | cut over; legacy deleted | Platform resolves the sibling package through its pnpm workspace.                                    |
| `packages/js/nuxt-runtime`       | `js/packages/nuxt-runtime`    | cut over; legacy deleted | Explicit server/client Adapter boundary retained.                                                    |
| `packages/js/yueli-ui`           | `js/packages/ui`              | migrated and deepened    | Added the experimental Nuxt module and public `CollectionFrame`; not a directory-for-directory copy. |
| `apps/http-runtime-conformance`  | `js/conformance/http-runtime` | cut over; legacy deleted | Production build and Playwright pass independently.                                                  |
| `apps/ui-foundation-conformance` | `js/conformance/ui`           | cut over; legacy deleted | Consumes public `YCollectionFrame`; no workspace source-scanning escape hatch.                       |
| `apps/ui-preview`                | `js/apps/ui-lab`              | curated and deleted      | Only promoted, product-neutral Patterns entered the Lab; the old Platform Preview was removed.       |
| `packages/go/gokit/authjwt`      | `go/auth`, `go/jwks`          | cut over; legacy deleted | Platform controllers use Foundation core through thin deployment and GoFrame policy adapters.        |

## Preview disposition

The old Preview imports `@platform/manage`, `@platform/ui` and `@platform/asset`, and includes company management-shell assumptions. Copying it wholesale would make the public repository depend on the platform it is meant to replace.

The Foundation UI Lab therefore consumes only public `@yueli/*` exports. Collection is present now. Dashboard, Settings, Feedback, BackToTop, media and user-menu examples enter only after their Interface, caller-owned i18n, tarball styling and real-consumer gates pass. Throwaway visual variants, fixture registries and debug screenshots are not migrated.

Platform now consumes Foundation through the explicitly temporary sibling overrides documented in [Local consumption](local-consumption.md). The migrated duplicate packages, conformance apps, Preview, Problem contract and `gokit/authjwt` implementation have been deleted after their affected product gates passed. Published versions will replace the overrides later; duplicate source must not return.
