# `@yueli/nuxt-runtime`

`@yueli/nuxt-runtime` 是 `@yueli/http-runtime` 的 Nuxt 4 适配层，把具名同源 BFF 路径转换为请求级
`ApiClient`，同时保证下游服务地址、凭据和产品端点不会进入浏览器配置。

消费者必须显式安装同一兼容线的 `@yueli/http-runtime` peer dependency；独立仓库应同时锁定两个不可变
GitHub Release tarball，不能依赖 npm registry 的同名包。

## 配置

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

target path 必须是同源绝对路径。模块会拒绝绝对 URL、协议相对路径、反斜杠、fragment、query 和 `..`
segment。只有 target 名称和路径进入 public runtime config；SSR cookie/header 策略保留在 server 私有配置。

## 调用

```ts
import { useApi } from "@yueli/nuxt-runtime/runtime";

const content = useApi();
const identity = useApi("identity");
```

`useApi` 为每个 target 创建一个缓存客户端。浏览器应用共享 app 级缓存，每次 SSR render 使用独立请求上下文。
HTTP Problem 会转换成只包含机器安全字段的 Nuxt error。

## BFF

```ts
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

模块负责下游路径校验、header 过滤、超时、响应 cookie 抑制和默认 1 MiB 请求体上限。应用仍负责私有 origin、
会话凭据、登录/刷新策略和产品 DTO。大文件直传应使用 Asset 的预签名协议，不应放宽通用缓冲 BFF。

## 边界与验证

- 不拥有下游服务地址、认证会话、产品端点、翻译或错误文案；
- 只允许受控 cookie/header 进入 SSR 请求，不转发浏览器 Authorization 和 forwarding header；
- 修改 runtime config、allowlist 或 BFF 行为时必须运行 `pnpm --filter @yueli/nuxt-runtime typecheck` 和
  `pnpm --filter @yueli/nuxt-runtime test`。
