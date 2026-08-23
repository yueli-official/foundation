# @yueli/ui

Experimental public UI foundation for Nuxt UI consumers.

The package intentionally has no root export. Import a documented explicit subpath so experimental modules can mature independently:

```ts
import { createCollectionWorkflow } from "@yueli/ui/collection";
```

Current public surface:

- `@yueli/ui` — Nuxt module registering experimental public components with a configurable `Y` prefix.
- `@yueli/ui/tailwind.css` — explicit Tailwind source declaration for raw package SFCs; import it from the consumer's Tailwind entry stylesheet.
- `@yueli/ui/theme` — provider-neutral Nuxt UI preset and complete Tabler icon contract.
- `@yueli/ui/theme.css` — opt-in light/dark semantic surfaces, motion/radius tokens, focus fallback and native scrollbar treatment.
- `@yueli/ui/manifest` — machine-readable maturity and ownership metadata.
- `@yueli/ui/messages` — caller-owned message key/parameter contract; no locale catalogs.
- `@yueli/ui/image` — bounded raster optimization/crop geometry, output naming and caller-translatable dimension validation.
- `@yueli/ui/image/browser` — disposable browser decode/canvas Adapter with explicit optimization/fallback results.
- `@yueli/ui/account-menu/pattern` — provider-neutral account/appearance grouping, identity fallback and inline/sidebar/collapsed menu trigger.
- `@yueli/ui/admin` — complete AdminConsoleLayout, lower-level shell/page primitives, shared PageHeader and TabbedSurface with caller-owned brand content, navigation, data and translation.
- `@yueli/ui/remote-select` — debounced, abortable and cached remote entity search built on Nuxt UI SelectMenu.
- `@yueli/ui/dashboard/pattern` — legacy compatibility entry for PageHeader, TabbedSurface and region-driven DashboardLayout; new admin consumers use `@yueli/ui/admin`.
- `@yueli/ui/feedback` — latest-wins action lifecycle, minimum loading visibility and transport-neutral notice normalization.
- `@yueli/ui/feedback/pattern` — explicit `ActionFeedbackButton` import.
- `@yueli/ui/navigation/back-to-top` — explicit accessible BackToTop Pattern import.
- `@yueli/ui/settings` — framework-independent JSON-safe baseline, dirty, capture and discard workflow.
- `@yueli/ui/settings/vue` — reactive Vue Adapter for a caller-owned settings form.
- `@yueli/ui/settings/browser` and `@yueli/ui/settings/vue-router` — opt-in unload and route-leave guards.
- `@yueli/ui/settings/pattern` — SettingsLayout, SettingSection and SettingsSaveDock explicit imports.
- `@yueli/ui/collection` — framework-independent remote collection workflow, schema-driven route query codec and memory query Adapter.
- `@yueli/ui/collection/vue` — Vue setup lifecycle, reactive snapshot and data-query invalidation Adapter.
- `@yueli/ui/collection/vue-router` — optional Vue Router query Adapter plus a reactive query-only composable for host-owned data loaders.
- `@yueli/ui/collection/pattern` — complete `CollectionPanel`, lower-level `CollectionFrame`, adaptive table toolbar, and composable Toolbar/Pagination/Dock/Footer/Tabs/View/selection patterns.

Enable the Nuxt module and package Tailwind source:

```ts
export default defineNuxtConfig({
  modules: ["@nuxt/ui", "@yueli/ui"],
  yueliUi: {
    // Only names that can arrive from persisted data or an API belong here.
    // Literal i-tabler-* names in application and @yueli source are scanned.
    tablerIcons: ["i-tabler-palette", "i-tabler-photo"],
  },
});
```

The module owns deterministic local Tabler delivery for every consumer: it
pins the Nuxt Icon peer, bundles the Foundation core icons plus scanned source
icons and the finite `tablerIcons` allowlist, and disables runtime API fallback.
Persisted icon values must be validated against the same application-owned
allowlist; do not add the whole Tabler collection to the client bundle.

### Upgrading to 0.2.0

Install the pinned `@nuxt/icon@2.4.1` and `@iconify-json/tabler@1.2.35` peers,
then list every Tabler name that can arrive from persisted data in
`yueliUi.tablerIcons`. Runtime icon API fallback is intentionally disabled, so
an unlisted dynamic name is a contract error instead of a late network fetch.

```css
@import "tailwindcss";
@import "@nuxt/ui";
@import "@yueli/ui/tailwind.css";
@import "@yueli/ui/theme.css";
```

Build the app config from a caller-owned color name. Foundation knows neither
the product name nor how its Tailwind palette is registered:

```ts
import { createUiPreset } from "@yueli/ui/theme";

export default defineAppConfig(createUiPreset({ primary: "brand" }));
```

The CSS surface API uses the `--yueli-*` and `.yueli-*` namespaces. Prefer
Tailwind utilities inside components; the stylesheet exists for cross-tree
tokens and browser-level behavior that cannot be expressed reliably by local
component utilities. It is opt-in so a consumer may use only the workflow
packages without adopting the visual theme.

Image policy is split from the browser runtime. The pure entrypoint rejects
active SVG input, preserves animated GIF by default, caps side length and total
canvas pixels, and exposes validation data instead of translated sentences.
The browser Adapter guarantees decoded object URL cleanup and can discard an
encoded result that is larger than the source:

```ts
import { optimizeImage } from "@yueli/ui/image/browser";

const result = await optimizeImage(file, { maxSide: 1920 });
// result.reason is stable; the caller decides whether/how to notify the user.
await upload(result.file);
```

The module auto-imports public Patterns with the `Y` prefix by default. It also auto-imports `useActionFeedback` and `useMinimumLoading`; consumers that prefer explicit imports can use `@yueli/ui/feedback`. BackToTop owns its scroll threshold, focus return, reduced-motion behavior and dock/overlay avoidance. Action feedback owns latest-wins async state and reset timing. AdminConsoleLayout follows Nuxt UI's official dashboard ownership and hides the fixed sidebar, search, active navigation, route bar, canvas, mobile drawer and back-to-top anatomy behind one small Interface. AccountMenu owns identity fallback, action grouping, optional appearance selection, sidebar trigger anatomy and the async logout command boundary without reading an auth provider or persisting color-mode state. Visible copy and appearance state remain caller-owned: pass translated props or messages and a preference Adapter; Foundation ships no locale catalogs.

Use the admin template with Nuxt UI primitives and caller-owned domain content:

```vue
<YAdminConsoleLayout
  brand-label="Content"
  brand-icon="i-tabler-file-text"
  brand-to="/"
  :navigation="navigation"
  :search-groups="searchGroups"
  :messages="shellMessages"
  storage-key="content-admin"
  main-id="content-main"
>
  <template #account="{ collapsed }">
    <ProductAccountMenu :collapsed />
  </template>
  <DocumentManagePage />
</YAdminConsoleLayout>
```

`YAdminConsoleLayout` is SSR-safe and is the server-rendered application shell. Do not
wrap the whole component in `ClientOnly` or replace it with a hydration-only
"opening console" screen. Keep browser-only behavior at the smallest child
boundary. Consumer acceptance should inspect the navigation response HTML for
`data-admin-shell` and verify that repeated authenticated refreshes never
render a whole-page placeholder.

Admin navigation is intentionally shallow. The public contract accepts flat
navigation or one parent level with leaf children; deeper product structure
belongs in page tabs, local navigation or breadcrumbs. A parent with children
should normally use `type: "trigger"`, while each child owns the actual route.
Product skins set semantic `--yueli-admin-shell`, `--yueli-admin-canvas` and
`--yueli-admin-search` tokens; the shared module owns geometry, states and
responsive behavior. `YAdminShell` and its typed `ui` prop remain a lower-level
seam only for a genuinely different console anatomy.
Normalize the caller-computed active state before rendering, and derive command
palette leaves from the same permission-filtered tree:

```ts
import {
  createAdminNavigationSearchItems,
  normalizeAdminNavigation,
  type AdminNavigationItem,
} from "@yueli/ui/admin";

const navigation = normalizeAdminNavigation([
  {
    label: "Site settings",
    icon: "i-tabler-settings",
    type: "trigger",
    children: [
      { label: "Home", to: "/manage/home", active: true },
      { label: "Footer", to: "/manage/footer" },
    ],
  },
] satisfies readonly AdminNavigationItem[]);

const searchItems = createAdminNavigationSearchItems(navigation, {
  idPrefix: "manage-page",
});
```

`normalizeAdminNavigation` promotes an explicitly active child to its parent
and opens that group on first render. It also makes every parent trigger-only,
discarding parent navigation and selection callbacks so a mobile disclosure
cannot close the Sidebar. Routes and authorization remain
caller-owned: recursively remove unauthorized leaves before normalization, then
use the same result for the Sidebar and search projection. Parent triggers do
not close a mobile Sidebar; leaf navigation may close it after selection.

Do not use the admin template as a generic card or analytics layout. Nuxt UI continues to own Button, Card, Table, Select and other primitives; products own metrics, permissions, routes and business actions. The deprecated DashboardLayout fixed four business regions and is not the target architecture. The structure follows the official [Nuxt UI dashboard template](https://github.com/nuxt-ui-templates/dashboard) without copying its fixtures or product pages.

`AdminPage` makes its `mainId` element the page scroll container and focus target. Pass the same ID to `BackToTop` as both `target-id` and `scroll-container-id`; sticky page content can also use that stable boundary without inspecting Nuxt UI's internal DOM.

Use `RemoteSelect` only when options come from a remote loader. Static and locally filtered options should use `USelectMenu` directly:

```vue
<script setup lang="ts">
import type {
  RemoteSelectLoader,
  RemoteSelectMessages,
} from "@yueli/ui/remote-select";

const authorId = ref<string | number | null>(null);
const messages: RemoteSelectMessages = {
  placeholder: "Select author",
  searchPlaceholder: "Search authors",
  empty: "No authors found",
  error: "Authors could not be loaded",
  retry: "Retry",
  minimumQuery: (count) => `Enter at least ${count} characters`,
};
const load: RemoteSelectLoader = async ({ query, signal }) => {
  const result = await api.searchAuthors({ query, signal });
  return {
    items: result.items.map((author) => ({
      value: author.id,
      label: author.name,
      description: author.email,
    })),
  };
};
</script>

<template>
  <YRemoteSelect v-model="authorId" :load :messages :minimum-query-length="2" />
</template>
```

The loader receives an `AbortSignal`; consumers must pass it to their HTTP adapter. The Pattern owns debounce, latest-wins sequencing, request cancellation, per-instance query caching and retry. It never displays raw transport errors.

Settings keeps persistence and translation outside the library. The pure workflow owns a cloned baseline and dirty/capture/discard semantics; Vue, browser unload and Router leave protection are separate opt-in Adapters. Route confirmation is a caller function, so Foundation never hard-codes a language or calls `window.confirm` on behalf of every product. The visible Patterns accept caller-owned labels and slots while standardizing responsive section navigation and the safe-area-aware save dock.
When a settings workflow is placed inside `AdminPage`, pass `:show-header="false"` to `SettingsLayout`; the page navbar remains the single page heading while settings sections keep their own local headings.

```vue
<script setup lang="ts">
import { useActionFeedback } from "@yueli/ui/feedback";

const save = useActionFeedback();
</script>

<template>
  <YActionFeedbackButton
    :status="save.status.value"
    idle-label="Save"
    pending-label="Saving"
    success-label="Saved"
    @click="save.run(persist)"
  />
  <YBackToTop label="Back to top" />
</template>
```

`CollectionPanel` is the default complete Pattern for card, grid and lightweight row collections: it owns responsive search, a single filter Popover, configured direction utilities, page and result selection, loading/error/empty states, row/grid containers and pagination. At the standard `@xl` container size (576px), search, the filter trigger and utilities share one row; compact containers put search first and the controls on a deterministic second row. Selection replaces that default toolbar in the same position instead of appending a sticky layer, while the column header remains visible. Callers may pass an empty `searchAction` to use live search, provide an `active-filters` slot for individually removable chips, and use a control `searchPlaceholder` to opt into Nuxt UI's searchable `USelectMenu`. Callers still own translated `CollectionPanelMessages`, query semantics, business HTTP and mutations. `isSelectable` on the Workflow and `isItemSelectable` on the Pattern express rows such as the current administrator that must remain visible but cannot enter batch selection. Because page-local eligibility cannot describe unloaded rows, all-results selection is intentionally unavailable when that predicate is configured. For strict data-table alignment, sorting, visibility and row selection, compose the public Collection Workflow with Nuxt UI `UTable` directly; the Foundation does not reimplement or shallow-wrap its column model. `CollectionTableToolbar` supplies the same toolbar anatomy around that direct `UTable`. Filter fields stay flat unless a real semantic distinction justifies sections under the [Collection toolbar standard](src/collection/toolbar-standard.md); strict table sorting remains in column headers. `CollectionFrame` remains the lower-level anatomy seam. Lightweight or domain-shaped screens may compose `CollectionToolbar`, `CollectionPagination`, `CollectionDock`, `CollectionFooter`, `CollectionLifecycleTabs`, `CollectionViewToggle`, `CollectionActiveFilters`, `CollectionPageSelection`, `CollectionRowShell` and `CollectionSortDirectionButton`.

Nuxt UI owns primitives. Collection patterns compose Nuxt UI controls only where the package adds stable responsive anatomy, query/selection semantics or accessibility behavior; it does not re-export generic buttons, cards or tables. DashboardLayout is a workflow-level composition of caller-owned regions, not a generic card/dashboard primitive.

Route query semantics stay caller-owned, while the shared codec applies the same normalization rules in every consumer:

```ts
const codec = createCollectionRouteQueryCodec({
  q: { kind: "string", default: "", maxLength: 120 },
  status: {
    kind: "enum",
    values: ["all", "draft", "published"] as const,
    default: "all",
  },
  page: { kind: "positive-integer", default: 1 },
});
```

Defaults are omitted from the URL. Repeated query values use the first value; invalid enums and positive integers fall back to the caller's declared default.

`useVueCollectionWorkflow` creates and disposes the Workflow inside Vue setup, binds an optional query Adapter, runs the first load on mount and exposes a reactive snapshot. A caller-owned primitive `dataQueryKey` can exclude presentation-only fields such as `view`, so list/grid changes do not refetch identical data. The caller's async `load` still owns business HTTP and must resolve or reject the supplied Workflow load token.

When Nuxt `useAsyncData` or another host loader already owns fetching, `useVueRouterCollectionQuery` supplies normalized reactive URL state and `replace`/`update` commands without introducing a second loader.

Run `pnpm --filter @yueli/ui test`, `pnpm --filter @yueli/ui typecheck` and
`pnpm --filter @yueli/ui test:pack` from the repository root. The pack conformance checks the
tarball allowlist, installs it into a temporary standalone Nuxt 4 consumer and runs a production build.

## 维护说明

- 生命周期：实验性公共 UI foundation，未达到 Pattern 门槛前不承诺稳定视觉 API。
- 权威来源：显式公开子路径、maturity manifest、单元测试及 UI conformance consumer。
- 维护边界：共享查询、选择、加载时序与消息引用；Nuxt UI primitives、业务字段、翻译和产品动作由调用方拥有。
- 变更要求：公开接口或路由同步行为变化时，必须同步更新单测、生产模式 Playwright 和 Work 证据。
