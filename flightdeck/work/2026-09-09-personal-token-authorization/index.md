# 个人令牌授权基础能力

Status: Open

## Goal
提供可复用的个人令牌认证与有限授权，Identity 管理凭证，消费站点保留角色和资源权限真值。首个消费者为 Blog，本地验收通过后再推进部署。

## Current
2026-09-09 已按用户授权部署相关 Provider、Blog 与 Docs；Identity sync-2、Account/Asset/Blog/Docs sync-1，生产目录/令牌生命周期验证通过，0035 已迁移。依赖使用固定私有候选包及独立 Foundation 快照，GitHub 正式 Release 尚未发布。证据见 acceptance.md 最后一节。
创建成功直接关闭表单回列表，不再自动弹出原文确认框。分类详情使用 Nuxt UI UPopover hover 模式卡片，含标题、数量和逐项权限，鼠标进入卡片后保持打开；用户进一步要求删除查看：现仅保留复制与撤销，原文弹窗及复制失败时弹出原文的分支全部移除。类型检查与真实 Playwright 创建/悬停/复制/撤销及无查看按钮、无原文弹窗验证通过。

按用户要求补充 300ms 搜索防抖；列表显示分类与总权限数，悬停查看权限。新 PAT 以 AES-GCM 加密保存，支持本人会话下随时复制/查看；已有仅摘要 Token 无法恢复。新增 owner-only POST /api/v1/pat/{id}/reveal、no-store 与审计。真实刷新后复制原令牌、查看、PAT 凭证读取拒绝、撤销后读取拒绝通过。

创建界面已按用户反馈改为展开的单个权限卡片：分类标题与操作列表、分类全选、全部全选、搜索、部分选中状态和已选数量；桌面双列、手机单列。搜索只影响显示，批量操作包含该范围内全部权限。2026-09-09 本次 Account typecheck、真实 CLI Playwright 搜索/批选/创建/撤销及浅深色桌面手机检查通过。

本地实现与真实组合验收完成：Foundation 共享认证/目录，Identity 按站点选择与提交校验，Blog 原文章 API 和受限媒体接入。复用 scopes JSON；新增 0035_pat_recovery 迁移，为 PAT 添加加密原文字段。未提交 Git、未发布正式 GitHub 制品；已完成本次私有候选生产部署。

Go 相关测试、Account typecheck、BFF 4 项测试、Identity 83 / Blog 70 项 HTTP 合同检查通过。CLI Playwright 八组屏幕/主题（模拟 API）和真实 Account 创建撤销通过。真实 API 26 项通过，包含上传完成、发布/下架、越权和撤销拒绝；真实 PostgreSQL 管理员撤权后同一 Token 编辑他人文章 200→403，再申请旧权限 400。

## Next
正式验收入口：https://account.yuelili.com/developer-tokens 。固定私有候选构建与生产更新已完成；后续按正式依赖发布 Work 发布 GitHub 制品。本地入口仍为 http://account-blog.dev.yuelili.test:3600/developer-tokens 。

当前隔离组合 20260909T062744Z-48004 正在运行，Blog http://blog.dev.yuelili.test:3002 。原 shared Blog 已停止，其他共享服务未停。隔离 Asset 原合同缺少 blog backend，本次用进程环境 GF_ASSET_BACKENDS 声明 local/blog 两个本地 Backend；未修改 Workspace 合同或生成文件。重启参数见 [验收记录](acceptance.md)。

## References
- [上下文](context.md)
- [Foundation 授权模块](../../../go/authorization/README.md)
