# `@yueli/nuxt-runtime`

Nuxt 4 Adapter for `@yueli/http-runtime`. It turns named same-origin BFF paths into request-scoped
`ApiClient` instances while keeping downstream origins, credentials and product endpoints outside
the browser interface.

The package is still private during Batch A conformance.

## Configure

```ts
export default defineNuxtConfig({
  modules: ["@yueli/nuxt-runtime"],
  yueliRuntime: {
    defaultTarget: "content",
    targets: {
      content: {
        path: "/api/bff/content",
        ssr: {
          cookies: ["session"],
          headers: ["accept-language"],
        },
      },
      identity: { path: "/api/bff/identity" },
    },
  },
});
```

Target paths must be same-origin absolute paths. Absolute URLs, protocol-relative paths,
backslashes, fragments, query strings and `..` segments are rejected during Nuxt setup.

Only target names and paths enter public runtime config. SSR cookie/header policies stay in private
server runtime config. Supported forwarded request headers are deliberately limited to
`accept-language` and `user-agent`; Authorization, forwarding headers and non-allowlisted cookies
are never copied into the internal BFF request.

## Use

```ts
import { useApi } from "@yueli/nuxt-runtime/runtime";

const api = useApi();
const identity = useApi("identity");
```

`useApi` is an explicit import rather than an unprefixed auto-import. It returns one cached client
per named target inside the current Nuxt app. Browser apps therefore keep one cache, while each SSR
render receives a separate cache and request context.

Core failures are converted to Nuxt errors containing only machine-safe fields:

```ts
createError({
  statusCode,
  statusMessage: failure.code,
  data: { failure },
});
```

This lets `getApiFailure(error)` work after SSR hydration without making backend `detail` or
exception text user-visible.

## Add an H3 BFF route

The server export owns downstream path validation, header filtering, timeout handling and response
cookie suppression. The app still owns the private origin and credential implementation:

```ts
// server/api/bff/content/[...path].ts
import { createBffHandler } from "@yueli/nuxt-runtime/server";

export default createBffHandler({
  mountPath: "/api/bff/content",
  resolveTarget: ({ event }) => ({
    origin: useRuntimeConfig(event).contentOrigin,
    pathPrefix: "/internal/v1",
  }),
  credential: {
    resolve: async ({ event, signal }) => {
      const token = await resolveAppOwnedToken(event, signal);
      return token ? { kind: "bearer", token } : { kind: "anonymous" };
    },
  },
});
```

`resolveTarget` must read private server configuration. Its result is validated as an HTTP(S)
origin without credentials, path, query or fragment. Browser input never selects an origin, and
downstream redirects are returned without being followed.

Set `profile: "asset"` for image/download routes. The response body remains streamed and the
handler additionally permits vetted byte-range and download metadata headers; browser credentials,
storage headers and downstream cookies remain blocked. Upload bodies are still bounded and buffered,
so large uploads should use direct presigned URLs or a separately reviewed streaming-upload adapter.

The default downstream request allowlist is `accept`, `accept-language`, `content-type`,
`idempotency-key`, `if-match`, `if-none-match` and `prefer`. Browser Cookie, Authorization,
forwarding and trace-control headers are discarded; the credential Adapter can only select
anonymous or inject one Bearer token. Responses retain only cache/content metadata,
`www-authenticate` and `x-trace-id`; downstream `set-cookie`, `location` and internal headers are
discarded. This preserves RFC 9457 status, body, content type and trace correlation without
exposing the downstream topology.

The handler defaults to a 1 MiB buffered request-body limit and one 10 second deadline covering
target resolution, credential resolution and downstream I/O. Both values are configurable.
Encoded request bodies and methods outside GET/HEAD/POST/PUT/PATCH/DELETE/OPTIONS are rejected.
Streaming uploads are intentionally a separate future Adapter rather than an option that weakens
the buffered handler's limit.

## Does not own

- downstream service origin values or product credential implementations;
- auth session/refresh policy or trusted client-IP forwarding;
- product endpoint definitions or DTOs;
- login redirect/session refresh implementation;
- translations, UI or error copy.

Those capabilities attach through app-owned Adapters rather than widening `useApi`.

## 维护说明

- 生命周期：实验性 Nuxt 4 Adapter，完成跨应用验证前保持私有。
- 权威来源：本包模块、runtime/server 公开子路径及 HTTP conformance consumer。
- 维护边界：只负责同源 BFF、SSR 请求上下文和安全转发；服务地址、凭证和产品 DTO 由应用负责。
- 变更要求：runtime config、header/cookie allowlist 或 BFF 行为变化时，必须同步更新安全测试和生产构建验证。
