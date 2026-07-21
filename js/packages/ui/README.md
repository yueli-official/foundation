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
- `@yueli/ui/collection` — framework-independent remote collection workflow, schema-driven route query codec and memory query Adapter.
- `@yueli/ui/collection/vue` — Vue setup lifecycle, reactive snapshot and data-query invalidation Adapter.
- `@yueli/ui/collection/vue-router` — optional Vue Router query Adapter.
- `@yueli/ui/collection/pattern` — explicit `CollectionFrame` import for consumers that do not use Nuxt component auto-import.

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

The module auto-imports `CollectionFrame` as `YCollectionFrame` by default. The experimental pattern owns only the integrated frame, responsive controls disclosure, sticky bulk region and accessible regions. Search/filter controls, domain columns/items, pagination, actions and all translated copy remain caller-owned slots.

Nuxt UI owns primitives. This package does not re-export or wrap buttons, tables, pagination, cards, tabs, or dashboards.

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
