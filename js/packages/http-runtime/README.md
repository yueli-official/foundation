# `@yueli/http-runtime`

Framework-neutral JSON HTTP state machine for the public site foundation. The package is public beta and has no runtime dependencies.

## Owns

- one `ApiClient.request` behavior entry;
- ordinary JSON success and RFC 9457 failure decoding;
- safe `ApiFailure` normalization and hydration-friendly extraction;
- relative-path and caller-header validation;
- query/JSON body construction, timeout/abort and response limits;
- refreshable-auth single-flight with at most one safe replay;
- an in-memory Transport Adapter under `@yueli/http-runtime/testing`.
- pure failure feedback resolution with field-level RFC 6901 violation projection, safe localized fallback, and code/trace details kept separate from primary copy.

## Does not own

- business endpoints, DTOs or service discovery;
- Nuxt request context, cookie forwarding or downstream credentials;
- authentication implementation, redirects or session storage;
- translations, toast/UI behavior or raw backend messages;
- downloads, media streams, SSE or WebSocket transport.

## Example

```ts
const api = createApiClient({
  target: "commerce",
  transport,
  reauth,
  refreshableAuthCodes: ["auth.session_expired"],
});

const order = await api.request<Order>("/api/v1/orders", {
  method: "POST",
  body: input,
  auth: "required",
});
```

Mutation replay defaults to `never`. To request one replay, the caller must set both
`replayAfterReauth: "once"` and an `idempotencyKey`; FormData mutations remain non-replayable.
Display text must come from the caller's resolver:

```ts
const failure = getApiFailure(error);
const message = failure
  ? resolveMessage(failure.code, failure.params)
  : resolveMessage("foundation.unknown", {});
```

Run `pnpm --filter @yueli/http-runtime test` and
`pnpm --filter @yueli/http-runtime typecheck` from the repository root.

## 维护说明

- 生命周期：public beta HTTP runtime；兼容新增按 minor 发布，不兼容 Interface 调整必须提供迁移说明。
- 权威来源：本包公开子路径、类型定义和单元测试。
- 维护边界：只维护框架无关的 JSON/Problem 状态机；业务端点、认证实现、翻译和 UI 由调用方负责。
- 变更要求：请求、重放、失败归一化或安全限制变化时，必须同步更新测试与 conformance consumer。
