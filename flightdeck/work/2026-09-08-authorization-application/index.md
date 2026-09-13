# 统一权限申请组件

## Goal

共享权限申请条目与用户资料展示，配合 Identity 公开资料读取，先在 Gallery 本地接入验证。

## Status

Finished

## Current

2026-09-09 来源标签统一：新增 AuthorizationGrantBadge，移除 Blog/Docs/Gallery 分散来源字典，Resource/Shop 不再直接输出原始 source。角色名使用中性 UBadge，中文来源进入 hover/focus 提示，包含 Foundation 全部 10 种来源。知识已写入 authorization-grant-badges.md。共享类型检查通过；Blog/Docs 固定 grants.1 包独立安装、typecheck/build 通过，Web 已上线 server-20260909-grants-1；正式组件 chunk SHA256 匹配、公开页 Playwright 通过。真实本地 Blog（initial_claim）、Docs、Gallery、Resource 的桌面/手机、hover/focus 验证通过。Shop 构建通过，首次两次隔离启动在 OAuth readiness 400 失败，继续用显式基础 OIDC scopes 验证；未修改授权规则。

Foundation UI 新增 AuthorizationUser 和 AuthorizationApplication，从现有 admin 导出；Identity Nuxt 新增批量公开资料目录；Gallery 申请列表及已授权用户接入。无权限规则、API 或数据库变更。

## Next

角色标签实现与 Blog/Docs 部署已完成，正式 Foundation Release 未执行。Shop 本地 OAuth readiness 400 阻断真实页面验收，基础 scopes 覆盖仍失败，保持构建通过与页面未验证的边界；失败组合已自动停止。

## References

- [上下文](context.md)

## Verification

- Foundation UI vue-tsc、Gallery Nuxt typecheck/build 通过。
- Identity 公开资料目录回归2项通过：items正确解析、101个去重主体分两批、请求失败传播。
- Foundation 仓库 CLI Playwright `js/packages/ui/test/authorization-gallery.acceptance.mjs` 通过：真实 Identity 昵称、Account 主页跳转、1440/390 布局与截图、批准/拒绝事件和忙碌禁用、三次刷新、503资料加载错误与重试恢复，无 pageerror。申请及审批接口为拦截夹具，没有产生实际授权；其余使用本地真实服务。初次断言因账户菜单同名定位失败，已改为已授权用户区域后通过。
- 证据 E:/tmp/yueli-authorization-shared/report.json 与 applications-1440.png、applications-390.png；手机截图已目检。
- 本轮无数据库/API修改，无部署、Git提交或推送。已有本地媒体改动保留，Gallery继续运行3007供用户验收。新公共API为增量导出，发布边界已记录在UI/Identity Nuxt README；当前使用Workspace local overlay。
