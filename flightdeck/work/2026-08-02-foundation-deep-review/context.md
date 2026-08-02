# 上下文

## 已知事实

- Workspace 锁定的 Foundation 版本为 Go `go/v0.1.0`、JS `js-v0.4.1`。
- Foundation 原本相对 `origin/main` 为 ahead 1、behind 16；已通过普通 merge 合入 `origin/main`，保留双方历史，没有 rebase 或强制改写。
- 本地已有提交 `667c78d feat(auth): require explicit subject kinds`。
- 已核验现有 Release：Go 为 `go/v0.1.0`，JS 最新为 `js-v0.4.1`；JS 当前通过 GitHub Release 完整包集合交付，不是 npm registry 发布。
- Platform 侧已有若干单模块实施/审计记录，但 Foundation 仓库此前没有仓库级 Flightdeck 深度 Review。

## 本轮稳定决策

- Go 保持单 module，使用 `go/vX.Y.Z` 子目录标签；当前没有拆 module 的复杂度收益。
- JS Release 使用 `js-vX.Y.Z` 标签并携带六个公开包的完整集合；包自身版本独立递增，未变化包允许保持原版本。
- access token 必须走 `auth.NewAccessTokenVerifier`，要求显式 audience、`at+jwt` 类型和 JWK `alg` 绑定；`auth.NewVerifier` 只用于 step-up 或自定义 JWT。
- 当前消费者按 GitHub Release tarball 接入；若迁移 npm，必须先构建 JavaScript 与声明文件，不能直接发布 raw TypeScript exports。
- 涉及 Web 的公共组件验收必须使用 CLI Playwright。

## 现有未提交修改

- `js/apps/ui-lab/app/app.vue`
- `js/conformance/ui/app/app.vue`

这些文件视为用户工作，不读取为本轮意图，也不修改、不暂存、不提交。

## Review 视角

1. 公共模块是否形成“接口小、能力深、复杂度藏在模块内”的边界。
2. Go module 与 npm package 的版本、标签、发布物、来源证明和兼容承诺是否一致。
3. Auth、HTTP、遥测等横切能力的默认值是否安全，错误语义是否稳定。
4. 契约、实现、conformance 和真实消费者是否构成闭环。
5. 新网站/App 能否仅依赖公开文档完成选型、安装、配置、升级和排障。
