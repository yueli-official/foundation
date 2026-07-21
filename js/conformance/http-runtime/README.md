# HTTP runtime Nuxt conformance consumer

Minimal Nuxt 4 SSR app that consumes `@yueli/http-runtime` only through the public
`@yueli/nuxt-runtime` Adapter. It verifies named same-origin routing, raw JSON success,
hydration-safe Problem errors, request-scoped SSR cookie/header allowlists and the public H3 BFF
handler.

The `/api/bff/proxy/**` route resolves an environment-owned origin and reaches a local downstream
route through real HTTP. It verifies server-owned Bearer injection, browser credential/forwarding
header removal and downstream `set-cookie` suppression. These routes are conformance fixtures, not
production endpoints or an auth implementation.

Verification:

```sh
pnpm --filter @yueli/http-runtime-conformance typecheck
pnpm --filter @yueli/http-runtime-conformance build
pnpm --filter @yueli/http-runtime-conformance test:e2e
```

## 维护说明

- 生命周期：HTTP runtime 的契约验证应用，不作为产品部署。
- 权威来源：`@yueli/http-runtime` 与 `@yueli/nuxt-runtime` 的公开导出。
- 维护者：平台 HTTP runtime 维护者；夹具只用于验证 SSR、BFF 和安全边界。
- 变更要求：公开接口、Problem 结构或凭证转发策略变化时，必须同步更新本应用及其 Playwright 验证。
