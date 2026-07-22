# @yueli/ui

Experimental public UI foundation for Nuxt UI consumers.

The package intentionally has no root export. Import a documented explicit subpath so experimental modules can mature independently:

```ts
import { createCollectionWorkflow } from "@yueli/ui/collection";
```

Current public surface:

- `@yueli/ui` — Nuxt module registering experimental public components with a configurable `Y` prefix.
- `@yueli/ui/tailwind.css` — explicit Tailwind source declaration for raw package SFCs; import it from the consumer's Tailwind entry stylesheet.
- `@yueli/ui/manifest` — machine-readable maturity and ownership metadata.
- `@yueli/ui/messages` — caller-owned message key/parameter contract; no locale catalogs.
- `@yueli/ui/account-menu/pattern` — provider-neutral account action grouping, identity fallback and accessible menu trigger.
- `@yueli/ui/dashboard/pattern` — PageHeader and slots-driven DashboardLayout with caller-owned section messages.
- `@yueli/ui/feedback` — latest-wins action lifecycle, minimum loading visibility and transport-neutral notice normalization.
- `@yueli/ui/feedback/pattern` — explicit `ActionFeedbackButton` import.
- `@yueli/ui/navigation/back-to-top` — explicit accessible BackToTop Pattern import.
- `@yueli/ui/settings` — framework-independent JSON-safe baseline, dirty, capture and discard workflow.
- `@yueli/ui/settings/vue` — reactive Vue Adapter for a caller-owned settings form.
- `@yueli/ui/settings/browser` and `@yueli/ui/settings/vue-router` — opt-in unload and route-leave guards.
- `@yueli/ui/settings/pattern` — SettingsLayout, SettingSection and SettingsSaveDock explicit imports.
- `@yueli/ui/collection` — framework-independent remote collection workflow, schema-driven route query codec and memory query Adapter.
- `@yueli/ui/collection/vue` — Vue setup lifecycle, reactive snapshot and data-query invalidation Adapter.
- `@yueli/ui/collection/vue-router` — optional Vue Router query Adapter.
- `@yueli/ui/collection/pattern` — explicit `CollectionFrame` and full `CollectionPanel` imports for consumers that do not use Nuxt component auto-import.

Enable the Nuxt module and package Tailwind source:

```ts
export default defineNuxtConfig({
  modules: ["@nuxt/ui", "@yueli/ui"],
});
```

```css
@import "tailwindcss";
@import "@nuxt/ui";
@import "@yueli/ui/tailwind.css";
```

The module auto-imports public Patterns with the `Y` prefix by default. BackToTop owns its scroll threshold, focus return, reduced-motion behavior and dock/overlay avoidance. Action feedback owns latest-wins async state and reset timing. PageHeader and DashboardLayout own responsive heading/action anatomy, region order and accessible section labelling without owning metrics or business actions. AccountMenu owns identity fallback, action grouping and the async logout command boundary without reading an auth provider. Visible copy remains caller-owned: pass translated props or messages; Foundation ships no locale catalogs.

Settings keeps persistence and translation outside the library. The pure workflow owns a cloned baseline and dirty/capture/discard semantics; Vue, browser unload and Router leave protection are separate opt-in Adapters. Route confirmation is a caller function, so Foundation never hard-codes a language or calls `window.confirm` on behalf of every product. The visible Patterns accept caller-owned labels and slots while standardizing responsive section navigation and the safe-area-aware save dock.

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

`CollectionPanel` is the default complete Pattern: it owns responsive search, configured select/direction controls, page and result selection, sticky bulk actions, loading/error/empty states, row/grid containers and pagination. Callers provide translated `CollectionPanelMessages`, query control values, items and domain slots; business HTTP and mutations remain outside the Module. `CollectionFrame` remains available as the lower-level anatomy seam.

Nuxt UI owns primitives. This package does not re-export or wrap buttons, tables, pagination, cards or tabs. DashboardLayout is a workflow-level composition of caller-owned regions, not a generic card/dashboard primitive.

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

Run `pnpm --filter @yueli/ui test`, `pnpm --filter @yueli/ui typecheck` and
`pnpm --filter @yueli/ui test:pack` from the repository root. The pack conformance checks the
tarball allowlist, installs it into a temporary standalone Nuxt 4 consumer and runs a production build.

## 维护说明

- 生命周期：实验性公共 UI foundation，未达到 Pattern 门槛前不承诺稳定视觉 API。
- 权威来源：显式公开子路径、maturity manifest、单元测试及 UI conformance consumer。
- 维护边界：共享查询、选择、加载时序与消息引用；Nuxt UI primitives、业务字段、翻译和产品动作由调用方拥有。
- 变更要求：公开接口或路由同步行为变化时，必须同步更新单测、生产模式 Playwright 和 Work 证据。
