# Foundation 公共模块、兼容性与发布实践调研

> 调研日期：2026-08-02
> 本地基线：合并后的 `1ea489f`；未提交工作区修改不作为“已发布能力”
> 范围：Go/JS 公共模块、发布供应链、JWT/OIDC、OpenTelemetry、conformance 与消费者契约
> 资料原则：只采用 Go、npm、Node.js、TypeScript、GitHub、OpenSSF、SLSA、IETF、OpenID Foundation、OpenTelemetry 与 Pact 的一手文档/规范。

## 结论摘要

| 判断                      | Foundation 现状                                                                                                                                                          | 结论                                                                                                                                                   | 优先级  |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------ | ------- |
| Go 子目录版本标签         | Go module 位于 `go/`，公开标签为 `go/v0.1.0`                                                                                                                             | **正确**。Go 官方要求子目录 module 的版本标签带 module 根目录前缀。                                                                                    | 保持    |
| Go module 粒度            | `go/` 下只有一个 `go.mod`，十余个领域包共享版本和完整依赖图                                                                                                              | 单 module 合法且维护最简单，但与“各模块独立依赖、独立 SemVer”不一致；应明确选择单一 Foundation Go module，或把重依赖/独立节奏边界拆成 nested modules。 | P1 决策 |
| JS 发布模型               | 六个 package 都是 `0.1.0`；npm 公共 registry 均不可见；GitHub Release 使用 `js-v0.1.0` 到 `js-v0.4.1` 的另一套集合版本                                                   | 当前是“GitHub Release tarball 集合”，不是常规 npm 公共包发布。集合版本与 package 版本双轨且未形成可靠映射。                                            | P0      |
| JS 产物正规性             | 大多数 `exports` 直接指向 `src/*.ts`；只有 `@yueli/ui` 有 packed-consumer gate，其他 conformance 仍用 `workspace:*`                                                      | `exports` 边界本身是对的，但 raw TypeScript tarball 不是通用 Node package；必须发布 JS + `.d.ts`，并让每个 package 从真实 tarball 验收。               | P0      |
| 发布自动化                | 当前唯一 workflow 在 `js-v*` 标签上只打包 `discovery-nuxt`，版本文件名硬编码；无安装、测试、构建、消费者验收、provenance；Actions 只钉 major tag；顶层 `contents: write` | 不能证明“标签对应的全部产物已由该提交通过验证并安全生成”。                                                                                             | P0      |
| 供应链维护                | 未见 Dependabot、dependency review、Scorecard、`govulncheck` 或 artifact attestation gate                                                                                | 对公共基础库不足；先补最低权限、依赖审查、漏洞扫描、不可变发布与来源证明。                                                                             | P0/P1   |
| JWT access-token verifier | 已做算法 allowlist、拒绝对称算法/`none`、强制 issuer/exp、固定 HTTPS JWKS、边界与并发保护                                                                                | 基础较强；但 audience 与 `typ` 可为空，且 JWK 的 `alg` 元数据在解析后丢失，尚未完整满足 RFC 8725 / RFC 9068 的 access-token profile。                  | P0      |
| OpenTelemetry             | Provider/Exporter 由调用方提供，安装全局状态是显式动作，并有出进程前清洗                                                                                                 | 方向好；但 SDK 装配和公共库计装应分层，span name 有语义约定偏差，substring denylist 不能代替数据最小化和 allowlist。                                   | P1      |
| Conformance               | 已有 Go adapter conformance、JS 单元/类型/构建、CLI Playwright，`@yueli/ui` 还有真实 tarball 安装构建                                                                    | 基础明显高于普通内部库；主要缺口是“所有发布包都从最终产物验收”和“与上一版本自动比较公共 API”。                                                         | P0/P1   |

## 仓库与发布现状快照

### Go

- module path 是 `github.com/yueli-official/foundation/go`，仓库只有一个 [`go/go.mod`](../../../../go/go.mod)。
- `go list -m -versions github.com/yueli-official/foundation/go` 返回 `v0.1.0`；`@latest` 也解析为 `v0.1.0`，标签时间为 2026-07-27。
- `go/abuse`、`go/audit`、`go/authorization` 等目录下的 `module.json` 是 Foundation 自己的能力清单，**不是** Go module 边界；Go 工具只识别 `go.mod`。
- 单一 `go.mod` 同时声明 GoFrame、Casbin、PostgreSQL driver、go-jose、OpenTelemetry API/SDK/contrib 等依赖。即使消费者只 import `auth`，模块级版本、漏洞清单与依赖治理仍覆盖整棵 module graph。
- `v0.1.0` 之后的 `SubjectKind` 强制校验属于可观察的 access-token 合同变化；即使 v0 允许破坏兼容，也应进入新的版本、迁移说明和消费者 gate，不能只停留在未标记提交。

### JavaScript / TypeScript

- 当前 package：`@yueli/content-nuxt`、`@yueli/discovery-nuxt`、`@yueli/http-runtime`、`@yueli/nuxt-runtime`、`@yueli/site-profile`、`@yueli/ui`，版本均为 `0.1.0`。
- 2026-08-02 匿名查询 npm registry 时，六个 package 均返回 `E404`；因此它们不是任何人可通过 npm 公共 registry 正常解析的公共包。
- 仓库已有公开 [GitHub Releases](https://github.com/yueli-official/foundation/releases)：
  - [`js-v0.3.0`](https://github.com/yueli-official/foundation/releases/tag/js-v0.3.0) 包含五个 `0.1.0.tgz`；
  - [`js-v0.4.1`](https://github.com/yueli-official/foundation/releases/tag/js-v0.4.1) 只包含 `yueli-discovery-nuxt-0.1.0.tgz`。
- 当前 [`release-js.yaml`](../../../../.github/workflows/release-js.yaml) 对所有 `js-v*` 标签只运行一个 `discovery-nuxt` job，且 tarball 文件名硬编码为 `0.1.0`。这既不是“每 package 独立发布”，也不是“同一集合标签完整发布全部 package”。
- package 已使用显式 `exports`，这是正确方向；但除少数预生成文件外，入口直接指向 `.ts` 源码。
- `@yueli/ui` 的 `test:pack` 会打包、安装到独立临时消费者并执行 Nuxt production build，这是值得扩展到所有 package 的好基线。现有 `js/conformance/*` 则主要通过 `workspace:*` 运行，无法发现漏文件、错误 exports、安装期 peer dependency 和 registry/tarball 差异。

## 1. Go 公共 module：版本、兼容与拆分

### 官方事实

1. Go 官方说明：单仓单 module 最简单；如果代码确实需要独立版本，也可以在一个仓库放多个 module。每个 module 根目录必须有自己的 `go.mod`，子目录 module 的标签必须带目录前缀，例如 `module1/v1.2.3`。[Go：Managing module source](https://go.dev/doc/modules/managing-source)
2. Go 版本号传达稳定性和兼容性：`v0.x.x` 表示仍在开发且不提供兼容保证；`v1+` 才是稳定承诺；minor 用于向后兼容的公共 API 增量，patch 不应改变公共 API 或依赖。[Go：Module version numbering](https://go.dev/doc/modules/version-numbers)
3. Go 官方 release workflow 仍要求 v0 持续发布新版本，并在 v1 才作正式稳定承诺；“v0 可破坏兼容”不等于“不需要版本、测试和迁移信息”。[Go：Module release and versioning workflow](https://go.dev/doc/modules/release-workflow)
4. `retract` 用于已发布过早或发现严重问题的版本；被 retract 的版本仍应保留，使旧构建不被破坏，同时阻止自动升级到该版本。[Go Modules Reference：`retract`](https://go.dev/ref/mod#go-mod-file-retract)
5. Go 团队的 `apidiff` 可比较两个 package/module 的导出 API，把变化分类为 compatible / incompatible；它不能判断行为变化，所以不能替代消费者测试。[`golang.org/x/exp/apidiff`](https://pkg.go.dev/golang.org/x/exp/apidiff)
6. 向公开 interface 直接加方法是破坏性变化；官方建议在可行时返回 concrete type，或增加新的可选 interface 并动态检测，而不是扩张旧 interface。[Go Blog：Keeping Your Modules Compatible](https://go.dev/blog/module-compatibility)

### 对 Foundation 的判断

#### 已符合

- `go/` 是 module 根，因此 `go/v0.1.0` 的标签形态完全符合官方规则。
- 当前处于 v0，可继续进行必要的边界调整，不需要伪造 v1 兼容承诺。
- 多数包使用“ordinary Go core + 显式 adapter”而不是框架全局，这是可拆分、可测试的良好前提。

#### 需要明确的架构选择

Foundation README 目前同时表达了“Go module”和“公共 Modules 独立依赖/发布”的意图，但真实发布单元只有一个 `github.com/yueli-official/foundation/go`。两种模式都可以，不能继续模糊：

**模式 A：保留一个 Go module。**

- 明确所有 Go package 共享一个 SemVer、一个 changelog、一个 compatibility window。
- 新增任何 public package 或依赖都由整个 module 的 minor version 表达。
- 优点是简单；缺点是 auth、telemetry、authorization、GoFrame、PostgreSQL 等不同风险和依赖节奏耦合。

**模式 B：有限拆分 nested modules。**

- 只按重依赖和独立发布节奏拆，例如评估 `go/telemetry`、`go/goframe`、大型持久化/策略模块；不要机械地“一目录一 module”。
- module path 保持现有 import path，例如 `github.com/yueli-official/foundation/go/telemetry`，对应标签为 `go/telemetry/vX.Y.Z`。
- 拆分前必须用 `go list -deps` 检查环，并给消费者补 `require` 迁移；`apidiff` 官方文档也明确指出，nested module 化即使 import path 不变，消费者的 `go.mod` 仍可能需要变化。

**建议：P1 做一次依赖图/发布节奏 ADR，再决定。** 当前尚不足以把“拆 module”列为 P0 修复；单 module 本身不是错误。但在决定前，文档不得再把自定义 `module.json` 暗示为独立 Go 发布单元。

### 建议 gate

- 每个 PR：`go test -race ./...`、`go vet ./...`、`govulncheck ./...`。
- 每次准备发版：用 `apidiff` 对比上一个 tag；incompatible 结果必须由版本/迁移说明显式接受。
- 每个真正由消费者实现的 public interface：保留共享 conformance suite，并至少由一个 Memory/reference adapter 与一个真实 adapter 执行。
- 误发严重版本时新增更高版本并 `retract`，不要删除/移动旧 tag。

## 2. npm 公共 package：版本、dist-tag、provenance 与 exports

### 官方事实

1. npm 的 `dist-tag` 是 SemVer 之外的可变人类标签；默认 `npm publish` 会把版本放到 `latest`，预发布可用 `beta`、`next` 等。npm 明确建议 dist-tag 不要以数字或 `v` 开头。[npm：Adding dist-tags](https://docs.npmjs.com/adding-dist-tags-to-packages/)
2. scoped package 要成为公共包，应按公共 scoped package 流程发布；官方要求发布前检查内容并安装测试，发布时使用 public access。[npm：Creating and publishing scoped public packages](https://docs.npmjs.com/creating-and-publishing-scoped-public-packages/)
3. `private: true` 会让 npm 拒绝发布；`publishConfig` 可以固定 registry、access 和 tag，避免发布到错误目标。[npm：package.json](https://docs.npmjs.com/cli/configuring-npm/package-json/)
4. npm trusted publishing 使用 CI OIDC 短期凭证，避免长期 token；GitHub Actions/GitLab trusted publishing 会自动生成 npm provenance。npm 当前要求 npm CLI 11.5.1+，且 `repository.url` 必须与授权仓库精确匹配。[npm：Trusted publishing](https://docs.npmjs.com/trusted-publishers/)
5. Node 推荐新 package 使用 `exports`，它既支持多个/条件入口，也封闭未声明的内部路径；新增 `exports` 可能是破坏性变化，因此首次发布前把所有承诺入口一次梳理好。[Node.js：Package entry points](https://nodejs.org/api/packages.html#package-entry-points)
6. Node 的原生 type stripping 明确**拒绝处理 `node_modules` 下的 TypeScript 文件**，以阻止 package 作者直接发布 TypeScript 源码作为 Node 依赖运行入口。[Node.js：Type stripping in dependencies](https://nodejs.org/api/typescript.html#type-stripping-in-dependencies)
7. TypeScript 官方把 `.ts` 定义为产生 JS 的实现文件，把 `.d.ts` 定义为只供类型检查的声明文件；发布 typed package 时建议提供 `types`/对应声明入口。[TypeScript：Type Declarations](https://www.typescriptlang.org/docs/handbook/2/type-declarations)、[Publishing](https://www.typescriptlang.org/docs/handbook/declaration-files/publishing.html)

### 对 Foundation 的判断

#### `exports` 方向正确，但产物层不完整

显式 subpath exports 已经把 public surface 封住，这是成熟做法。问题在于入口大多指向 `src/*.ts`：

- Nuxt/Vite 消费者可能替 package 转译，因此现有 packed Nuxt build 能通过；
- 普通 Node/通用 ESM 消费者不能据此得到可移植保证；
- package 的 TypeScript 版本、transpilation target 和 bundler 能力被隐式转嫁给消费者；
- `.d.ts` API 与 runtime JS 没有独立产物，因此难以做稳定 API diff。

发布到 npm 之前应为每个 package 生成 `dist/*.js` 与 `dist/*.d.ts`，并使用条件 exports，例如同时声明 `types` 和 `import`。Nuxt module 若确实要求 Nuxt build-time 转译，也应把该限制写入 package contract，而不是把它当成所有 package 的默认模式。

#### 版本模型必须二选一

当前 `js-v0.4.1` 与 tarball 内 `0.1.0` 是两套 SemVer。建议：

- **首选：每 package 独立版本。** package 的 `version` 是消费者版本真相；Git tag 明确携带 package 身份；release notes 按 package 生成；npm dist-tag 只表达 `latest`/`next` 通道。
- **如果坚持集合发布：** 把 `js-v0.4.1` 明确定义为 release train/bundle ID，而不是 package 版本，并在 release manifest 中列出每个 package 的 name、version、sha256、source commit。集合标签必须发布清单声明的全部产物，不能像当前 `js-v0.4.1` 只发一个包却保留全局名称。

无论选哪种，README 的“独立 SemVer、changelog 与 release tag”必须与自动化真实行为一致。

### npm 发布前 P0 清单

- 所有 public package 使用真实非占位版本，固定 `publishConfig.access: public` 和预期 registry。
- `exports` 指向已构建 JS，并为 TypeScript 暴露对应 declarations；不把 raw `.ts` 当普通 Node runtime 入口。
- 每个 package 都有最小 `files` allowlist，`npm pack --dry-run --json`/真实 pack 的文件清单受测。
- 每个 tarball 安装到 `--ignore-workspace` 的干净消费者：至少运行 runtime import、typecheck、production build；UI/Nuxt 再运行 CLI Playwright。
- 首次公开稳定前冻结所有 subpath exports；后续移除/收紧 entrypoint 按 breaking change 处理。
- 预发布使用 `next`/`beta` dist-tag，不要把候选版本直接放入 `latest`。
- 采用 npm trusted publishing；配置后禁用传统长期 token，保留 provenance。

## 3. OpenSSF / SLSA / GitHub 发布供应链

### 官方事实

1. SLSA v1.2：Build L1 要求构建 provenance；L2 要求托管构建平台自己生成并签名 provenance，从而防止构建后篡改；provenance 只有经过验证才产生安全价值。[SLSA Build track](https://slsa.dev/spec/v1.2/build-track-basics)、[Verifying artifacts](https://slsa.dev/spec/v1.2/verifying-artifacts)
2. GitHub artifact attestation 可以把 artifact 绑定到源码和 workflow；immutable releases 会阻止 release asset/tag 在发布后修改或删除，并自动产生 release attestation。[GitHub：Supply chain security](https://docs.github.com/en/code-security/concepts/supply-chain-security/supply-chain-security)、[Immutable releases](https://docs.github.com/en/enterprise-cloud@latest/code-security/concepts/supply-chain-security/immutable-releases)
3. GitHub 官方指出，Action 只有钉到完整 commit SHA 才是不可变引用；组织/仓库也可强制该策略。[GitHub Actions：Secure use reference](https://docs.github.com/en/actions/reference/security/secure-use)
4. OpenSSF Scorecard 把依赖更新工具、Pinned Dependencies、Token Permissions、Vulnerabilities、Branch Protection 等作为公共项目安全健康检查；它建议 workflow 顶层只读、只在 job 层授予必要写权限。[OpenSSF Scorecard checks](https://github.com/ossf/scorecard/blob/main/docs/checks.md)
5. GitHub dependency review 能在 PR 中展示直接和间接依赖变化，并可让 workflow 在新引入已知漏洞时失败。[GitHub：Reviewing dependency changes](https://docs.github.com/en/pull-requests/how-tos/review-pull-requests/reviewing-dependency-changes-in-a-pull-request)
6. Go 官方的 `govulncheck` 按实际可达符号降低噪声，并明确支持接入 CI。[Go：Vulnerability Management](https://go.dev/doc/security/vuln/)、[Security Best Practices](https://go.dev/doc/security/best-practices)

### 对 Foundation 当前 workflow 的判断

当前 `release-js.yaml` 有以下高优先级问题：

- tag 触发后直接打包/创建公开 release，之前没有 install、lint、typecheck、unit、build、pack-consumer、Playwright gate；
- 只打一个 package，却用集合级 `js-v*` 名称；
- 文件名和 package version 硬编码，tag/version/package.json 不一致时不会失败；
- workflow 顶层 `contents: write`，超出 checkout/pack 阶段所需；
- `actions/checkout@v4`、`pnpm/action-setup@v4`、`actions/setup-node@v4` 都是可移动 tag，不是完整 SHA；
- 没有 artifact/npm provenance、SBOM 或发布 attestation；
- 没有 draft/staged + environment approval，也未证明 immutable releases 已启用；
- 当前无普通 PR CI、Dependabot、dependency review、Scorecard 与 Go 漏洞 gate。

### 推荐的发布控制

**P0：先建立最小可信链。**

1. PR CI 完成 Go/JS 全部验证；release job 通过 `needs` 只消费已通过的同一 commit。
2. workflow 顶层 `contents: read`；只有创建 release 的 job 授予 `contents: write`，npm trusted publishing job 另授 `id-token: write`，不共享长期 token。
3. 所有第三方 Actions 钉完整 SHA，并由 Dependabot 自动更新 SHA。
4. tag 创建受保护；发布 job 使用 GitHub environment，必要时 required reviewer 且禁止自审。
5. GitHub tarball 路线：先创建 draft、上传全部清单产物/校验信息、生成 attestation，再发布并启用 immutable release。
6. npm 路线：trusted publishing + provenance；高风险首发可用 npm staged publishing，由 2FA 审核后进入 registry。

**P1：持续供应链治理。**

- 对 npm、gomod、GitHub Actions 配 Dependabot；PR 执行 dependency review。
- 定期执行 OpenSSF Scorecard；把结果当改进信号，而不是单一“安全分数”。
- `govulncheck ./...` 与 JS registry audit/OSV 检查进入定时和 release gate。
- 为 GitHub Release 产物和未来非 npm 产物生成可验证 provenance；消费者文档提供验证命令。

## 4. JWT / OIDC 库侧验证

### 官方安全底线

1. RFC 8725 要求 verifier 由调用者指定允许算法集合，不得使用集合外算法；每个 key 必须绑定到恰好一种算法并在验证时检查。[RFC 8725 §3.1](https://www.rfc-editor.org/rfc/rfc8725.html#section-3.1)
2. verifier 必须验证 issuer 与其密钥的绑定；一个 issuer 面向多个应用时必须校验 audience；为避免不同 JWT 类型互相替代，推荐显式 `typ` 和互斥的 validation rules。[RFC 8725 §3.8–3.12](https://www.rfc-editor.org/rfc/rfc8725.html#section-3.8)
3. RFC 9068 的 OAuth JWT access-token profile 要求 signed token、`typ: at+jwt`（或 `application/at+jwt`）、`iss`、`exp`、`aud`、`sub`；resource server 必须精确匹配 issuer，并验证 audience 是自己的 resource indicator。[RFC 9068 §2/§4](https://www.rfc-editor.org/rfc/rfc9068.html)
4. RFC 8414 要求授权服务器 metadata 与 `jwks_uri` 使用 HTTPS，并规定字符串精确比较；JWKS 中同时包含签名和加密 key 时，每个 key 必须有 `use`。[RFC 8414](https://www.rfc-editor.org/rfc/rfc8414.html)
5. token 中的 `kid`、`jku`、`x5u` 等不能被当成可信查询/URL；尤其不能跟随 token 自带任意 URL，否则会产生注入或 SSRF。[RFC 8725 §3.10](https://www.rfc-editor.org/rfc/rfc8725.html#section-3.10)

### Foundation 已做对的部分

- `auth.Config` 使用显式算法 allowlist，默认 RS256，并只接受批准的非对称算法；不会接受 `none` 或 HS/RS 混淆。
- `issuer` 和 `exp` 强制存在；可限制 `exp-iat`；token/body/fetch 有大小和时间边界。
- `jwks.RemoteSource` 使用配置期固定 endpoint，不读取 token 自带 URL；生产默认仅 HTTPS，禁止 redirect，限制 body，合并并发 refresh，节流 unknown `kid`。
- JWKS 只接受 public signing key、跳过非 `sig` 用途、拒绝空/重复 `kid`；短暂上游故障时只给已知 stale key，不给未知 key。

### 必须补的合同

| 缺口                                                 | 风险                                                                                             | 建议                                                                                                                                                            | 优先级 |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ |
| `Audiences` 可为空                                   | 作为“access-token verifier”时会跳过资源服务器身份校验，ID token/发给其他 API 的 token 可能被接受 | access-token profile 强制至少一个 audience；如确需通用 JWT，另设名字和构造器，要求调用者显式选择 profile                                                        | P0     |
| `Types` 可为空                                       | 默认关闭 `typ` 检查，不能默认阻止 ID token / access token 混淆                                   | RFC 9068 profile 默认只接受 `at+jwt`/`application/at+jwt`；其他 token 类型使用互斥 profile                                                                      | P0     |
| `jwks.indexKeys` 丢弃 JWK `alg`，只返回裸 public key | token header 算法虽在 allowlist，但未验证它与 key 声明算法一致                                   | KeySource 返回带 `kid/use/alg/key` 的 typed verification key，验证 token `alg == JWK alg`；无 `alg` 时执行明确策略并记录文档                                    | P0     |
| issuer 与 JWKS 的绑定完全靠调用方手工配对            | 配置错配时可能用错误 issuer 的 key set                                                           | 保留显式配置，但提供可选的 metadata factory：从受信 issuer 配置出发获取 metadata，精确校验返回 issuer 和 HTTPS `jwks_uri`；绝不从 token claim/header 动态选 URL | P1     |
| `subject_kind` 是 Foundation 自定义 claim            | 它不是 OIDC/OAuth 标准 claim，生产者/消费者升级不一致会导致全部 token 被拒                       | 发布 Yueli access-token profile 文档/fixture：claim 名、允许值、`sub/client_id` 组合、版本和迁移；身份服务与至少一个真实资源服务共同 conformance                | P0/P1  |

建议把“通用 JWT 验证器”和“Yueli OAuth access-token verifier”做成两个深度不同的入口：底层复用解析/签名能力，上层 profile 一次性收紧 `typ`、iss、aud、exp、actor/custom claims。这样调用方不能靠遗漏配置意外降级安全策略。

## 5. OpenTelemetry 公共库指导

### 官方事实

1. OpenTelemetry 要求 API 与 SDK 解耦；第三方库/框架只依赖 API，最终应用决定是否安装和配置 SDK/exporter。未安装 SDK 时，API 应安全 no-op。[OpenTelemetry Client Design Principles](https://opentelemetry.io/docs/specs/otel/library-guidelines/)
2. instrumentation scope 通常用 instrumentation library 的全限定名称和版本唯一标识，并可带 schema URL。[OpenTelemetry：Instrumentation Scope](https://opentelemetry.io/docs/specs/otel/common/instrumentation-scope/)
3. HTTP span 名应为 `{method} {low-cardinality target}`；server target 优先使用 route template，不能默认使用具体 URI path。[OpenTelemetry HTTP semantic conventions](https://opentelemetry.io/docs/specs/semconv/http/http-spans/)
4. OpenTelemetry 无法替实现者判断业务上下文中的敏感数据；官方建议数据最小化，并用 attributes/filter/redaction/transform 等处理器做删除、散列或 allowlist。[OpenTelemetry：Handling sensitive data](https://opentelemetry.io/docs/security/handling-sensitive-data/)
5. 官方通常建议服务旁使用 Collector，以承接重试、批处理、加密和额外敏感数据过滤。[OpenTelemetry Collector](https://opentelemetry.io/docs/collector/)

### 对 Foundation 的判断

#### 正向设计

- `telemetry.NewProvider` 要求调用方提供 exporter 和 sampler，且不在 import 时安装全局状态。
- `InstallGlobal` 是显式动作；HTTP client 通过 clone 避免修改调用者 client。
- 所有 span 在离开进程前经过 sanitizing exporter，是有价值的最后一道边界。
- server span name 只接受 route template，不使用具体 path/query，避免高基数，这是正确方向。

#### 需要调整

1. **明确它是应用 SDK 装配包，不是普通 instrumentation API。** 当前 `telemetry` 直接依赖 SDK、resource、otelhttp 并构造 provider，这对 application integration helper 合理，但不应成为所有 Foundation 库计装的默认依赖。若未来其他公共 package 发 span，它们只依赖 OTel API，并用自己的 instrumentation scope；SDK 装配移到清晰的 `telemetry/sdk` 或 adapter 边界。P1。
2. **修正 HTTP span name。** 当前生成 `HTTP GET /route`，官方语义约定是 `GET /route`；无 route 时应是 `GET`（未知/非标准 method 才按约定归一）。P1，需更新测试与 dashboard 查询。
3. **denylist 不能被描述为完整隐私保证。** 当前按 key substring 删除 `token`、`cookie`、`db.statement` 等很有价值，但无法识别业务 PII，也可能因新 semantic attribute 漏掉。增加允许属性集/业务策略入口，在 source 与 Collector 两层执行数据最小化，并持续维护 redaction corpus。P1。
4. **标准环境读取要变成明确合同。** `resource.WithFromEnv()` 会读取标准 OTel resource 环境配置；这不是“公司私有环境变量”，但仍是隐式部署输入。文档应明确它，或改为调用方显式 opt-in，以符合 Foundation 的显式 process policy。P1。

## 6. Conformance、消费者契约与发布自动化

### 主流一手实践

- npm 官方建议发布前先安装实际 package；`npm pack` 可生成将要分发的 tarball。[npm pack](https://docs.npmjs.com/cli/v10/commands/npm-pack/)、[Scoped public package testing](https://docs.npmjs.com/creating-and-publishing-scoped-public-packages/)
- Go `apidiff` 用于导出 API 的静态兼容近似，但官方明确它无法发现行为变化，所以仍需真实消费者/行为测试。[Go apidiff](https://pkg.go.dev/golang.org/x/exp/apidiff)
- Pact 的 consumer-driven contract 模式是：消费者写出对 provider 的交互假设并生成 contract，provider 重放验证；它适合 HTTP/消息 provider 合同，不必用于普通进程内 library API。[Pact Provider Verification](https://docs.pact.io/implementation_guides/go/docs/provider)
- GitHub environment 可在发布 job 前要求 reviewer、禁止自审并限制允许发布的 branch/tag；保护规则通过前 secret 不会下发。[GitHub：Deployments and environments](https://docs.github.com/en/actions/reference/workflows-and-actions/deployments-and-environments)

### Foundation 应采用的三层 gate

| 层                         | 目的                                                | Foundation 实施                                                                                                                                 |
| -------------------------- | --------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| 静态/API 兼容              | 发现 export/import path、类型、interface 的编译破坏 | Go `apidiff` 对上一 tag；TS 生成 `.d.ts` 后做 API report/diff；校验 `exports` 清单                                                              |
| Artifact conformance       | 验证真正发布的文件，而非 workspace 源码             | 每个 npm/GitHub tarball 安装到干净目录，runtime import + typecheck + production build；Go 从临时消费者按目标 tag/pseudo-version 编译            |
| Consumer/provider contract | 验证真实使用语义和 wire contract                    | 至少一个真实站点/服务 fixture；HTTP/消息边界按需要使用 Pact 风格 consumer contract + provider replay；数据库 adapter 继续共享 conformance suite |

现有 `@yueli/ui test:pack` 是第二层的好实现，应抽成复用 harness 覆盖六个 package。现有 Playwright 与 Nuxt conformance 是第三层的一部分，但凡依赖 `workspace:*` 的测试都不能替代发布产物验收。

### 建议发布流水线

1. **PR 阶段**
   - frozen install；format/lint/typecheck/unit/build；
   - `go test -race`、`go vet`、`govulncheck`；
   - API diff 与 contract/fixture 校验；
   - 所有 package pack + isolated consumer；
   - Web/Nuxt 用 CLI Playwright 做页面、交互和必要截图验收；
   - dependency review。
2. **release candidate 阶段**
   - 从同一个受保护 commit 计算版本和 release manifest，不接受硬编码文件名；
   - 再次从最终 tarball 执行 smoke；
   - 生成 checksums、SBOM（若采用）和 provenance/attestation；
   - npm 用 `next` 或 staged publishing，GitHub 用 draft release。
3. **批准与发布**
   - environment reviewer 检查版本映射、changelog、迁移、产物清单；
   - 发布 npm 或完整 GitHub release；启用 immutable release；
   - 只在全部产物成功后移动 `latest`/完成 release，不留下“部分 package 成功”的集合版本。
4. **发布后验证**
   - `go list -m ...@latest`、`npm view name@version`/`npm install name@tag` 验证公共解析；
   - 验证 provenance/attestation；
   - 用一个独立真实消费者按公开版本完成最小 smoke。

## 优先级建议

### P0：下一次对外发布前

1. 决定 JS 是 npm 独立 package 还是 GitHub 集合 tarball，并统一 tag、package version、release 名称与清单。
2. 把所有 JS runtime entrypoint 构建为 JS + `.d.ts`；六个 package 全部执行真实 tarball isolated-consumer gate。
3. 建立 PR CI 和完整 release gate；去掉硬编码版本；最小权限；Actions 钉完整 SHA。
4. JWT access-token profile 默认强制 audience、`typ` 与 JWK algorithm binding；发布自定义 `subject_kind` 合同 fixture。
5. 发布说明准确列出已有 `go/v0.1.0` 与 JS GitHub releases，不再使用会让读者误判“尚无任何公开产物”的状态描述。

### P1：随后完成

1. ADR 决定 Go 单 module 或有限 nested modules；若保留单 module，明确共享 SemVer/依赖/兼容承诺。
2. 上线 Dependabot、dependency review、`govulncheck`、Scorecard 与 artifact attestation/immutable release。
3. 用 `apidiff`/TS declaration API report 自动对比上一发布版本，并要求 migration note 接受不兼容结果。
4. OpenTelemetry 分离 API instrumentation 与 SDK/app assembly；修正 span name；升级到数据最小化 + allowlist + Collector 二次清洗。

### P2：成熟化

1. 为受信 issuer 提供安全 metadata factory 和 exact issuer/JWKS 绑定，同时保持 URL 配置不受 token 控制。
2. 对多 package 发布加入原子化失败策略、版本提案 PR、release manifest 和自动 changelog。
3. 为跨服务 HTTP/消息合同引入按消费者驱动的 provider verification；不要把 Pact 强行用于普通 Go/TS 进程内 API。

## 最终适用判断

Foundation 的核心实现并非“基础能力都很差”：Go 的安全边界、adapter conformance、JS 的显式 exports、`@yueli/ui` packed consumer、CLI Playwright 都是扎实基础。当前最大的系统性短板在**公共产物层**：源码、版本、标签、tarball、registry、消费者验证与 provenance 还没有形成单一可追溯事实链。

因此，本轮重构优先级应是：先让“发布的东西”正规、可安装、可验证、可追溯；再决定 Go module 是否拆分。不要在发布链尚未闭合时先大规模移动实现目录。
