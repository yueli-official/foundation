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

Repository bootstrap is active. The HTTP/Nuxt runtimes, Collection workflow and experimental `CollectionFrame` have independent unit, type, production-build and browser conformance. `@yueli/ui` also passes a real packed-tarball standalone Nuxt build with package-owned Tailwind scanning. The Go Problem core, hardened JWKS source, strict JWT access-token verifier and GoFrame Problem/auth adapters pass canonical/real-server/concurrency tests, race tests and vet.

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
