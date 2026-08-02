# Foundation 深度 Review 结论

## 已完成

### P0：鉴权与密钥边界

- 新增 `auth.NewAccessTokenVerifier`，强制 audience、`at+jwt` 类型和 JWK 算法绑定。
- JWKS source 保留完整 `jose.JSONWebKey` 元数据；通用 verifier 在 JWK 声明算法时校验匹配，access token verifier 进一步要求算法不可缺失。
- 新增 `contracts/auth/access-token-profile.md` 与 Go 使用说明，明确通用 JWT 和 access token 不得混用构造器。

### P0：供应链与依赖安全

- 升级存在可达漏洞的 `x/text` 与 OpenTelemetry 依赖，并用精确 override 清除 JS 生产依赖漏洞。
- CI/Release Action 固定到 commit SHA；新增 Go race/vet/govulncheck、JS 全门禁、生产依赖审计、公共包 pack、独立消费者和 CLI Playwright。
- JS Release 改为六个公开包的完整集合，生成 v1 manifest、SHA256SUMS 和 build provenance；发布 job 使用最小权限与 `foundation-release` environment。
- 增加版本校验脚本，变化包未提升自身版本时拒绝发布。

### P1：公共 UI 语义与运行时行为

- Admin sidebar、主/次导航都有稳定可访问名；Admin Page 自身成为唯一顶层 `main` landmark。
- Settings 内部标题容器不再误成为嵌套 banner；Collection 分页得到可访问名。
- 批量操作区保持正常文档流，同时在主滚动容器内 sticky；CLI Playwright 验证滚动后仍位于页面工具栏下方。
- 浅色和深色稳定页面状态均通过 axe；远程选择、键盘操作、历史恢复和保存流程均通过真实页面验收。

### P1：消费者与发布文档

- `docs/consumer-integration.md` 给出新网站、App、CLI 和 Go 服务的安装、配置、升级和验收路径。
- `docs/release-policy.md` 固化 Go 单 module、JS full-bundle、包版本与标签的关系。
- README 和本地消费文档移除“尚未发布”等过时描述。

## 验收证据

- `pnpm verify:js`：格式、lint、类型、全部 JS 单测与构建通过。
- `go test -race ./...`、`go vet ./...`：通过。
- `govulncheck v1.6.0`：0 个可达漏洞。
- `pnpm audit --prod`：0 个已知漏洞。
- CLI Playwright：UI 9/9、HTTP runtime 3/3 通过。
- JS 发布门禁：六个公开包 dry-run、版本增量校验、UI packed consumer build 均通过。

## 后续事项

### 跨服务迁移（下一个 Go Release 前）

- Identity 与其他 access token 消费者需要切换到 `NewAccessTokenVerifier`，签发端需输出 `typ=at+jwt` 且 JWKS 必须声明与签名一致的 `alg`。Foundation 已提供严格能力和契约，但服务仓库迁移不属于本仓库实现边界。

### npm 发布前置条件

- 当前六个包仍通过 GitHub Release tarball 交付，npm registry 尚无这些包。
- 若决定发布 npm，先将 raw TypeScript exports 改为构建后的 JavaScript 与 `.d.ts`，再增加 npm provenance/可信发布；在此之前文档不得给出 `pnpm add @yueli/*` 的 registry 安装承诺。

### 仓库管理员设置

- 为 `foundation-release` environment 配置 required reviewers，并在 GitHub 仓库设置中启用适用的 immutable release/tag 保护。此类设置无法仅靠仓库文件完成。

### 已知 P1 技术债

- 解决 Nuxt 4.4.8、`@nuxt/kit` 4.5.x 与 `oxc-parser` 的 peer warning，再升级 Nuxt 版本族。
- 对 OpenTelemetry span 命名基数、属性隐私和日志/追踪关联做专项审计。
- 对 Nuxt 生产 chunk 超过 500 kB 的警告做按路由/组件拆包分析；当前不影响正确性门禁。
