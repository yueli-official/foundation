# UI foundation conformance

Blank Nuxt 4 consumer for the experimental `@yueli/ui` public Interface.

The visible collection page is deliberately a copy-owned Block in this app. It proves that the headless workflow can drive Nuxt UI without
turning the app page, fixtures, filters, columns, or domain actions into a public runtime export.

Current checks:

- `pnpm --filter @yueli/ui-foundation-conformance typecheck`
- `pnpm --filter @yueli/ui-foundation-conformance build`
- `pnpm --filter @yueli/ui-foundation-conformance test:e2e`

The Playwright suite runs against a production Nuxt server and covers controlled URL history, keyboard search and bulk selection, sticky bulk actions, mobile reflow, horizontal overflow, light/dark screenshots and axe checks.

Still required before any Pattern promotion:

- packed-tarball install instead of a workspace link;
- reduced-motion behavior once the Block introduces motion;
- a second real product composition that proves which visible behavior is shared.

## 维护说明

- 生命周期：实验性 UI foundation 契约应用，不作为产品部署。
- 权威来源：`@yueli/ui` 的公开接口与本应用的 copy-owned Collection Block。
- 维护者：共享前端基础设施维护者；业务字段、夹具和产品动作仍由消费方拥有。
- 变更要求：查询、选择、路由同步或可访问性契约变化时，必须同步更新单测和生产模式 Playwright 验证。
