# Yueli Foundation

Yueli Foundation 是面向 Yueli 各类站点、应用与其他程序的公开、版本化基础模块集合。
它不是平台仓库的镜像，也不是要求所有消费者采用同一框架的 mega-kit。

## Repository model

- `contracts/`：跨语言 wire contracts 与 conformance fixtures。
- `js/packages/`：framework-neutral TypeScript、Vue/Nuxt UI Pattern 与 Nuxt Adapter。
- `js/conformance/`：只通过公开 Interface 验证真实安装、SSR、浏览器与安全行为的最小消费者。
- `js/apps/`：文档和开发期 UI Lab，不拥有产品业务实现。
- `go/`：ordinary Go core 与显式 GoFrame Adapter；迁移前逐 Module review。
- `docs/`：架构、兼容矩阵、迁移和发布说明。

JS package 与 Go module 共用仓库和跨语言 contract，但拥有独立依赖与版本线；仓库没有统一版本。Go 当前以
`go/` 下的单一 module 发布，JS 当前以带清单和校验和的 GitHub Release 完整 bundle 交付，package 自身的
`version` 仍是消费者兼容性的版本真相。

## Current status

Repository bootstrap is active. The HTTP/Nuxt runtimes and public UI workflow/Pattern modules have independent unit, type, production-build and browser conformance. `@yueli/ui` also passes a real packed-tarball standalone Nuxt build with package-owned Tailwind scanning. Go `identifier` and JS `@yueli/identifier` now share versioned UUIDv7, UUIDv5 and Public Locator contracts without reimplementing UUID bit layouts. Go Foundation also includes Problem/errors, bounded HTTP decoding, health, hardened JWKS/JWT auth, privacy-bounded telemetry, explicit rate-limit/OpenAPI policy and their GoFrame adapters. The public Authorization Module adds typed instance-local Catalog, Scope, Grant, Group, Constraint, delegation, application/invitation, automatic rules, Policy Revisions, Scope-owned custom roles and query planning, backed by both a reference Adapter and normalized PostgreSQL truth with a derived Casbin v3 projection. Traffic adds privacy-bounded resource view counting, exact replay protection, daily visitor deduplication, typed aggregate queries and normalized PostgreSQL truth. Work adds transactional enqueue/outbox, delayed and recurring jobs, queue concurrency, leases, retry, progress, pause/cancel, attempt history and replay on the same consumer-owned PostgreSQL model. Discovery adds trusted-origin page projections, typed JSON-LD, streaming sitemap/index, RSS/Atom, robots, atomic short-TTL publication snapshots and a versioned Nuxt Adapter; Blog, Resource and Docs exercise Article, Product/Collection and hierarchical version/locale paths. The public Modules have Memory references and shared conformance where their Adapter model requires them. The complete Go module passes real-server/concurrency tests, race tests and vet.

URL Lifecycle adds query-aware stable Route identities, canonical/alias/redirect/gone state, temporary overlays, atomic subtree transitions, single-hop owner targets, revision/idempotency, deterministic archive/recovery, normalized PostgreSQL truth with caller-owned transaction binding, and a trusted-origin HTTP Adapter.

Site Profile adds a typed public-profile aggregate for one site instance,
stable icon/Asset references, scheduled announcements, support/footer/legal
material, optimistic revision, strong ETag, schema-driven management metadata,
normalized PostgreSQL truth and conditional HTTP reads/writes.

Webhook adds instance-local CloudEvents publication, Standard Webhooks
signatures and secret rotation, transaction-bound fan-out, durable delivery
and attempt history, Work-based retry/replay, inbound verification receipts,
and DNS-rebinding-resistant outbound HTTP.

已经发布的公共制品包括 Go `go/v0.1.0`，以及 JS `js-v0.1.0` 至 `js-v0.4.1` 的 GitHub Release tarball。
现有版本仍处于 `v0`，允许有迁移说明的接口调整，但不得绕过版本、制品验收或消费者迁移。不要直接依赖仓库源码路径。

Useful local gates:

```sh
pnpm verify:js
pnpm --filter @yueli/ui test:pack
pnpm --filter @yueli/ui-foundation-conformance test:e2e
cd go && go test -race ./... && go vet ./...
```

See [Local consumption](docs/local-consumption.md) for using the JS packages and Go module from a sibling checkout without copying source.
新站点、App 或服务应先阅读 [消费者接入指南](docs/consumer-integration.md)；维护者的版本与发布规则见
[发布策略](docs/release-policy.md)。

## License

Apache-2.0. Brand assets, private configuration, credentials and product content are not included.
