# 统一后台发布候选

本文件描述本地候选，不表示已经提交、打标签或发布。发布顺序为 Foundation → Registry/Hub/BVideo/Blog；消费者不得提前引用不存在的版本。

## 发布单元

| 单元                 | 现有正式基线 | 本地目标    | 内容                                                                                           |
| -------------------- | ------------ | ----------- | ---------------------------------------------------------------------------------------------- |
| Foundation Go module | `go/v0.4.1`  | `go/v0.5.0` | 新增 Classification adminapi；同时包含基线后已提交的个人令牌授权、授权发现和 HTTP 合同投影能力 |
| Foundation JS bundle | `js-v0.7.5`  | `js-v0.8.0` | 完整七包 bundle，其中 UI 升至 `0.5.0`，其他包保持既有版本                                      |

Go 和 UI 都新增公开能力，使用 minor 升版。JS bundle 编号与各 package SemVer 分开。远端 tag 列表已核对，两个目标 tag 尚不存在。

## 发布说明草案

Foundation UI 0.5.0 新增 `@yueli/ui/admin-platform`：typed Product Definition、基于 capability 的导航/搜索/当前页面投影，以及 Category、Tag、FacetValue 的树、搜索、分页和父级防环行为。Comments selection 增加可选 `isSelectable(id)`。已有 shell、Collection、Settings 和授权原语继续使用原公开入口，产品保留自身的数据、路由、权限和业务 mutation。

Foundation Go 0.5.0 新增 `classification/adminapi`，提供 Category、Facet、FacetValue、Tag、Policy 和 revision 的 HTTP/JSON 合同及领域转换。它不注册路由，不持有鉴权、数据库或事务。该 module 自 0.4.1 以来还包含已提交的个人令牌目录/Principal 和 Authorization 发现、HTTP contract project/CLI 等变化，消费者必须运行自己的鉴权与 HTTP 合同门禁。

## 消费者切换

1. Foundation Go 正式 tag 可经 Go proxy 下载后，在 Registry 更新 Foundation module 到 `v0.5.0`。把 `internal/adapters/httpapi/classification.go` 和其他实际引用处的 `internal/classificationwire` 改成正式 `classification/adminapi`，使用 `adminapi` 包名并删除内部镜像目录。保留 Category 的 `icon/imageMediaKey/contentCount` 强类型扩展和现有转换校验。
2. 执行 Registry 分类 HTTP 测试、完整 Go 门禁和生成合同检查，确认 JSON、revision 和数据库事务语义不变；不添加 `replace`、`go.work` 或旧接口兼容层。
3. Foundation JS Release 完整可下载后，Hub/BVideo/Blog 使用 manifest 列出的 UI 0.5.0 tarball，更新各自 package/override/lockfile。旧 0.4.0 候选只在没有引用后移除。
4. 从最终 tarball 完成消费者 typecheck/build 与真实 CLI Playwright；本地源码 overlay 通过不能代替正式制品消费验证。

## 新产品创建衔接

后台公共能力的创建入口是 Product Definition + 已有 Module + 产品 Adapter，详见[架构](../../../docs/admin-platform.md)。端到端创建还要覆盖独立仓库生成、能力选择、Identity/Asset 注册、本地组合、首位管理员、首个业务对象与发布交付。Workspace 现有 bootstrap 只负责物化锁定仓库，不是新产品生成器。

新产品创建流程需保持两类定义分离：Product Definition 描述后台拓扑；Environment 描述运行组合与能力绑定。网站、桌面软件和后端服务各自保留运行入口，按实际需求消费后台与平台服务。不能要求无后台的软件也生成一整套站点。

下一阶段先确定一个具体新产品旅程作为生成器验收样例，再把独立仓库、能力接入和本地运行落实为可重复执行的创建流程；不把示例网站或通用 CRUD 壳当作完整交付。

## 验证

2026-09-14 本地完成以下门禁：

- Go `go test -race ./...`、`go vet ./...`、`go mod tidy -diff`、`govulncheck@v1.6.0` 均通过；没有可达漏洞。
- JS frozen install、`pnpm verify:js`（format/lint/typecheck/unit/build）全部通过，UI 为 37 文件 / 126 测试。
- 七个公共包 pack dry-run 通过；UI `test:pack` 验证公开文件与 exports、独立 Nuxt 消费者 typecheck/production build 通过。
- OSV 2.5.0 扫描 pnpm lock 的 1193 个包，无问题。首次 TLS 超时后，仅为扫描子进程使用现有 Windows 系统代理，未改系统代理配置。
- Foundation CLI Playwright：UI 12 passed，另 2 项因未指定专用消费者而按原配置 skipped；HTTP conformance 3 passed。
- Blog 权限真实登录、申请与用户集合选择/取消、角色能力页和 390/768/1280px 检查 1 passed。Workspace Isolated Session 为 `20260914T141923Z-28820`。
- JS release validator 相对 `js-v0.7.5` 通过，确认只有 UI 包需要升版；`git diff --check` 通过。

本地证据：Foundation `.cache/admin-platform-{verify-js,pack,ui-e2e,http-e2e,osv-proxy,frozen-install}.log`；Blog `web/test-results/e2e/admin-platform-recovery-20260914/junit.xml` 与同目录 artifacts。

七包候选及 `SHA256SUMS` 位于 Foundation `.cache/admin-platform-release/`，UI 0.5.0 tarball SHA256 为 `d714b682f06aea05270cc9ec72f30081c9e0d1d78aa337e2c85dad9ab5843003`。候选来自未提交工作树；正式 manifest 必须等真实发布 commit 存在后生成，不能用当前 HEAD 冒充候选来源。

尚未执行：Git commit、push、tag、远端 CI/Release，以及 Registry/三个 Web 消费者的正式版本切换。远端 CI 必须重新通过发布门禁；本地制品与运行结果不替代正式下载验收。
