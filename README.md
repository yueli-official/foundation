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

JS package 与 Go module 共用仓库和跨语言 contract，但拥有独立依赖、CI、SemVer、changelog 与 release tag；
仓库没有统一版本。

## Current status

Repository bootstrap is active. The HTTP/Nuxt runtimes and public UI workflow/Pattern modules have independent unit, type, production-build and browser conformance. `@yueli/ui` also passes a real packed-tarball standalone Nuxt build with package-owned Tailwind scanning. Go Foundation now includes Problem/errors, bounded HTTP decoding, health, hardened JWKS/JWT auth, privacy-bounded telemetry, explicit rate-limit/OpenAPI policy and their GoFrame adapters. The public Authorization Module adds typed instance-local Catalog, Scope, Grant, Group, Constraint, delegation, application/invitation, automatic rules, Policy Revisions, Scope-owned custom roles and query planning, backed by both a reference Adapter and normalized PostgreSQL truth with a derived Casbin v3 projection. Traffic adds privacy-bounded resource view counting, exact replay protection, daily visitor deduplication, typed aggregate queries and normalized PostgreSQL truth. Work adds transactional enqueue/outbox, delayed and recurring jobs, queue concurrency, leases, retry, progress, pause/cancel, attempt history and replay on the same consumer-owned PostgreSQL model. Discovery adds trusted-origin page projections, typed JSON-LD, streaming sitemap/index, RSS/Atom, robots, atomic short-TTL publication snapshots and a versioned Nuxt Adapter; Blog, Resource and Docs exercise Article, Product/Collection and hierarchical version/locale paths. The public Modules have Memory references and shared conformance where their Adapter model requires them. The complete Go module passes real-server/concurrency tests, race tests and vet.

URL Lifecycle adds query-aware stable Route identities, canonical/alias/redirect/gone state, temporary overlays, atomic subtree transitions, single-hop owner targets, revision/idempotency, deterministic archive/recovery, normalized PostgreSQL truth with caller-owned transaction binding, and a trusted-origin HTTP Adapter.

Site Profile adds a typed public-profile aggregate for one site instance,
stable icon/Asset references, scheduled announcements, support/footer/legal
material, optimistic revision, strong ETag, schema-driven management metadata,
normalized PostgreSQL truth and conditional HTTP reads/writes.

Webhook adds instance-local CloudEvents publication, Standard Webhooks
signatures and secret rotation, transaction-bound fan-out, durable delivery
and attempt history, Work-based retry/replay, inbound verification receipts,
and DNS-rebinding-resistant outbound HTTP.

No prerelease has been published. Package names and compatibility remain experimental; do not consume repository source paths directly.

Useful local gates:

```sh
pnpm verify:js
pnpm --filter @yueli/ui test:pack
pnpm --filter @yueli/ui-foundation-conformance test:e2e
cd go && go test -race ./... && go vet ./...
```

See [Local consumption](docs/local-consumption.md) for using the JS packages and Go module from a sibling checkout without copying source.

## License

Apache-2.0. Brand assets, private configuration, credentials and product content are not included.
