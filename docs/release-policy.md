# Foundation 版本与发布策略

## 发布单元

Foundation 是 polyglot 仓库，不使用仓库统一版本。

- Go 当前保持一个 module：`github.com/yueli-official/foundation/go`。`module.json` 是能力成熟度/证据清单，不是独立 Go module，也不拥有独立版本。
- JS package 各自拥有 SemVer；`js-v*` 是 GitHub Release 完整 bundle 编号。bundle 必须列出全部公共 package 的 name、version、文件名和 SHA-256。
- 拆分 Go nested module 只有在依赖重量、发布节奏和消费者迁移收益均有证据时才通过 ADR 执行，不按目录机械拆分。

## 版本规则

- `v0` 允许不兼容调整，但仍必须升版、写迁移说明并通过真实消费者 gate。
- 已发布 package 内容发生任何变化都必须改变 package version；兼容修复使用 patch，兼容新增使用 minor。
- 删除 export、改变 Go public interface、收紧必填 contract 或改变 wire 语义均视为 breaking change。
- Go 严重误发使用更高版本配合 `retract`，不移动或删除旧标签。
- 不通过 fallback 同时长期维护未发布的旧接口；迁移在生产者与消费者验证后一次完成。

## PR 门禁

CI 必须执行：

- Go：`go mod tidy` diff、`go test -race ./...`、`go vet ./...`、`govulncheck ./...`；
- JS：frozen install、format/lint/typecheck/unit/build、生产依赖 audit、全部公共 package pack dry-run；
- 制品：至少一个独立 tarball 消费者；覆盖 Web 的 conformance 必须使用 CLI Playwright；
- Identifier：Go/JS conformance 向量、最终 tarball 隔离消费者和站群跨仓静态门禁；
- 发布相关 Action 使用完整 commit SHA，Dependabot 负责提出更新。

## JS bundle 发布

标签触发后先重复完整 JS gate，再执行：

1. 找到前一个 `js-v*` 标签；所有发生内容变化的 package 必须已升版；
2. 打包全部公共 package，不接受手写包名或硬编码版本文件名；
3. 生成 `foundation-js-release-manifest.v1.json` 与 `SHA256SUMS`；
4. 为最终资产生成 GitHub artifact attestation；
5. 先创建 draft，全部资产上传成功后再公开 Release。

仓库管理员还需在 GitHub 设置中启用 immutable releases、保护发布标签，并为 `foundation-release` environment 配置 reviewer。代码内 workflow 不能替代这些仓库级控制。

## npm 迁移条件

当前不声称这些 package 已发布到 npm。迁移到 npm trusted publishing 前必须同时满足：

- 普通 Node package 的 runtime export 指向构建后的 JS，并提供 `.d.ts`；
- 每个 package 都从最终 tarball 在隔离消费者中完成 import/typecheck/build；
- 配置 public access、OIDC trusted publishing 与 provenance；
- package SemVer/tag/dist-tag 成为单一可理解模型，消费者文档同步切换。
