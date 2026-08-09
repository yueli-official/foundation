# Foundation 消费者接入指南

本文面向在其他仓库中新建网站、App、CLI 或服务，并接入 Foundation 公共能力的团队。Foundation 不是必须整包采用的框架；只安装实际需要的 Go package 或 JS package。

## 1. 先选公开边界

| 需求                                      | 推荐入口                                      | 不应由 Foundation 承担          |
| ----------------------------------------- | --------------------------------------------- | ------------------------------- |
| Go HTTP Problem、JWT/JWKS、健康检查、遥测 | `github.com/yueli-official/foundation/go/...` | 业务 DTO、路由拓扑、域名、凭据  |
| Go 授权、审计、任务、Webhook 等领域模块   | 对应 ordinary-Go core；框架只接显式 adapter   | 产品数据库所有权、产品权限命名  |
| Go/JS 普通实体与公开定位符                | `go/identifier`、`@yueli/identifier`          | secret、Trace、幂等键、业务编号 |
| Nuxt BFF 与 Problem 解码                  | `@yueli/http-runtime`、`@yueli/nuxt-runtime`  | 私有 origin、cookie/凭据策略    |
| 公共 UI 工作流/Pattern                    | `@yueli/ui` 的显式 subpath export             | 产品文案、路由、权限和业务动作  |
| 内容、Discovery、Site Profile             | 对应 Nuxt adapter/package                     | 产品数据源和发布目标            |

优先依赖最窄的公开 export。不要从 `src/`、未声明 subpath、其他产品仓库或 Foundation 内部相对路径导入。

## 2. Go 正式制品

Foundation Go 当前是一个 module，所有 `go/` 下的 package 共享同一 SemVer。子目录 module 的正确标签带 `go/` 前缀：

```sh
go get github.com/yueli-official/foundation/go@v0.1.0
go list -m github.com/yueli-official/foundation/go
```

消费者 `go.mod` 中应出现：

```go
require github.com/yueli-official/foundation/go v0.1.0
```

正式 CI 不得依赖 sibling `replace` 或 `go.work`。升级时先阅读目标版本迁移说明，再运行消费者的单测、竞态测试、静态检查和真实服务 smoke。

### JWT 接入

通用、显式类型的签名 JWT 使用 `auth.NewVerifier`。OAuth resource server 在下一 Go 版本起应使用更严格的
`auth.NewAccessTokenVerifier`，它强制：

- 至少一个目标 audience；
- `typ` 为 `at+jwt` 或 `application/at+jwt`；
- JWK `alg` 存在且与 token header 的 `alg` 一致；
- issuer、expiry、签名算法 allowlist 和 Yueli `subject_kind` 合同。

生产者和消费者必须在同一迁移窗口切换，不能用“缺字段时猜测”的兜底策略。Yueli token 合同见
[Access Token Profile](../contracts/auth/access-token-profile.md)。

## 3. JS 正式制品

当前 JS 包通过 GitHub Release tarball 分发，不在公共 npm registry。package 内 `version` 是 API 兼容版本；
`js-v*` 是一次完整制品 bundle 的交付编号，不代替 package SemVer。

当前已验证的历史制品位置：

| Package                 | Package 版本 | Release     |
| ----------------------- | ------------ | ----------- |
| `@yueli/http-runtime`   | `0.1.0`      | `js-v0.2.0` |
| `@yueli/nuxt-runtime`   | `0.1.0`      | `js-v0.2.0` |
| `@yueli/site-profile`   | `0.1.0`      | `js-v0.2.0` |
| `@yueli/ui`             | `0.1.0`      | `js-v0.2.0` |
| `@yueli/content-nuxt`   | `0.1.0`      | `js-v0.3.0` |
| `@yueli/discovery-nuxt` | `0.1.0`      | `js-v0.4.1` |

`@yueli/identifier@0.1.0` 已在源码中完成，但尚未进入正式 JS Release；消费者不得自行拼接下载 URL。

示例：

```sh
pnpm add "https://github.com/yueli-official/foundation/releases/download/js-v0.2.0/yueli-http-runtime-0.1.0.tgz"
```

不要自行拼接尚未发布的 URL。下一次 bundle 起，Release 会同时包含七个公共 tarball（含
`@yueli/identifier`）、
`foundation-js-release-manifest.v1.json`、`SHA256SUMS` 和 GitHub artifact attestation；消费者以 Release 实际资产为准。

Nuxt 应用按各 package README 注册 module/CSS。服务端私有 origin 与凭据转发只放在消费者 runtime config；浏览器只看到公开 BFF 路径和目标名。

## 4. 本地联调

需要修改 Foundation 与消费者时，可按 [本地消费](local-consumption.md) 使用 sibling workspace 或 `go.work`。本地
override 必须留在开发环境，提交或发版前切回正式制品并重新安装。

## 5. 消费者验收清单

- 锁定明确的 Go module 版本或 JS tarball URL，不跟随分支源码。
- 检查 package export/module import 只使用公开入口。
- 运行消费者自身的 unit、typecheck、build 和生产配置 smoke。
- Web/Nuxt 的页面与交互必须使用仓库内 CLI Playwright 验收。
- HTTP 边界验证 raw success DTO 与 RFC 9457 Problem，不添加私有通用 envelope。
- Auth 同时验证 issuer、audience、type、algorithm、expiry 与 actor profile。
- 升级后删除临时兼容代码；未发布项目不保留旧接口兜底。

## 6. 升级与问题定位

升级时一次只改变一个 Foundation release 单元，并保留消费者锁文件 diff。若失败，先区分：

1. 安装/缺文件/exports 错误：属于制品 conformance；
2. 编译或类型错误：检查 package SemVer 与迁移说明；
3. HTTP/claims 行为错误：检查跨服务 contract 和生产者版本；
4. 仅本地成功：检查 sibling override、环境私有 origin 和未提交生成物。

安全问题按根目录 [SECURITY.md](../SECURITY.md) 的私有渠道报告。
