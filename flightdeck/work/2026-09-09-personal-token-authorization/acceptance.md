# 本地验收 2026-09-09

- Foundation auth / authorization / authorization/postgres 测试通过（本次未为 postgres 包指定专用集成 DB，其结果不能代表该包全部数据库场景）。新增目录查询测试确认发现后代能力不授予兄弟资源权利。
- Identity logic/controller/cmd、Blog blogauthz/controller/server/cmd、Asset runtime/upload/controller/cmd 测试通过；Account typecheck 和共享 BFF 4 项测试通过。
- Identity 82 operations、Blog 70 operations HTTP 合同检查无漂移。
- CLI Playwright：390/768/1024/1440 × light/dark，模拟 API 的界面选择/创建/撤销通过，无水平溢出，Axe serious/critical 为 0。截图已经检查；截图等待选择器关闭动画结束。
- CLI Playwright 真实 Account：创建包含 Blog 读取/创建/上传能力的令牌并撤销，通过，无 pageerror。
- 真实 API：26 项请求检查通过。创建、编辑、发布、下架、BFF 转发、上传初始化/PUT/完成；只读写入 403，仅发布夹带内容编辑 403，仅编辑下架 403，PAT 治理与签发拒绝；Asset 错误 profile 403，撤销后调用 401。上传使用部署允许的 GIF 类型。
- 真实数据库角色测试：临时测试用户获管理员，Token 编辑他人文章 200；通过业务接口撤销角色，同一 Token 再编辑 403；创建旧能力 Token 400。测试临时授权和 Token 已撤销，保留本地测试用户和文章样本。

证据目录 `E:/tmp/yueli-personal-token-20260909`：account-ui.json、live-ui.json、live-api.json、live-role.json；源码脚本在同目录。模拟与真实 API 证据分别标明，未保存明文 PAT。API 负向调用检查状态码，错误 DTO 由合同/单元测试覆盖。

## 隔离组合

通过 Workspace CLI `environments/blog-local/run.ps1 -Mode Isolated` 启动，session `20260909T020543Z-40952`。不改顶层合同，不手写 .doctor 状态。

本次进程环境：LOCAL_IDENTITY_PORT=8681、LOCAL_ACCOUNT_PORT=3600、LOCAL_ASSET_PORT=8682、LOCAL_BLOG_API_PORT=8085、LOCAL_BLOG_WEB_PORT=3002。

GF_PAT_APPLICATIONS 为 `[{"id":"blog-main-web","name":"月离博客","permissionsUrl":"http://127.0.0.1:8085/api/v1/internal/personal-token/permissions","audience":"blog-main-web"}]`。
GF_BLOG_PERSONALTOKENS_SITEID=blog-main-web。
GF_ASSET_PERSONALTOKENS_AUTHORITIES 为 `{"blog-main-web":"http://127.0.0.1:8085/api/v1/personal-token/media-authorization"}`。
GF_ASSET_BACKENDS 为 `{"local":{"type":"local","root":"./.data/assets"},"blog":{"type":"local","root":"./.data/blog-assets"}}`，补齐隔离注册声明引用的 blog backend。

## 发布边界

源码 overlay 验收通过；尚无新正式依赖制品/生产构建/线上验收。Go 消费者需固定含新共享 auth/authorization API 的 Foundation 版本，Blog Web 需固定含 PAT 转发的 Identity Nuxt 版本。生产配置要求见 integration.md。当前授权投影为每实例单进程，不能承诺多副本实时权限撤销。

## 权限卡片交互更新

移除站点多选下拉，所有分类展开在一个权限卡片内。搜索匹配分类、操作名称、说明与能力 key；不清空隐藏项。分类/全部全选作用于完整分类/目录，并显示半选态；目录更新才剔除已不可用权限。

Account typecheck 与目标文件 diff 检查通过。真实 CLI Playwright 验证分类全选、单项取消后的半选、全部选择、搜索后分类取消、搜索后全部选择、无结果与清除搜索、创建 3 项权限 Token 及撤销。390/1280 浅深色无横向溢出，Axe serious/critical 0；已检查桌面手机截图。证据 E:/tmp/yueli-personal-token-20260909/cards-ui.json、cards-ui.mjs、cards-{theme}-{width}.png。未部署生产。

## 搜索、紧凑摘要与令牌恢复

新增 0035_pat_recovery 通过 Workspace CLI 本地迁移，当前隔离组合 20260909T062744Z-48004。Identity HTTP 合同增为 83 个操作。PAT codec、logic、controller 测试通过：密文随机性、错误密钥/绑定/损坏拒绝、owner 隔离、过期、撤销与旧记录不可恢复。DAO 编译通过（无独立单测）。

真实 CLI Playwright：搜索输入 100ms 后仍保留原结果，停止输入后更新；分类摘要/总数与悬停详情；真实创建后刷新页面，复制内容与原 Token 完全相同，再次查看相同原文；无会话的 Bearer PAT 读取 401，撤销后读取 404；读取响应 no-store，列表不含 Token 原文和密文字段。浅深色 390/1280 界面检查无严重 Axe 问题，页面无异常。验收 Token 已撤销，用户自行创建的旧 test Token 未修改。

证据 `E:/tmp/yueli-personal-token-20260909/token-recovery-ui.json`、同名 mjs 与 compact-token-list.png；没有在输出/证据 JSON 中写入原文。旧 Token 无法恢复不是数据库迁移失败。

## 创建收尾与悬浮权限卡片

用户要求取消创建后的原文确认：创建成功返回列表，只有显式查看才打开原文。分类改为 Nuxt UI UPopover(mode=hover)，标题/数量/逐条权限；进入卡片不关闭。Account typecheck 与真实 CLI Playwright 验证创建后无弹窗、刷新、hover 三项权限并保持打开、复制/查看/撤销通过，截图已检查。证据 E:/tmp/yueli-personal-token-20260909/hover-card-ui.json、hover-card-ui.mjs、token-hover-card.png。

用户要求移除查看：已删除列表查看按钮、原文弹窗和复制失败显示原文的分支。真实 CLI Playwright 验证创建返回列表、刷新后复制、无查看按钮和弹窗、悬浮卡片、撤销通过。证据 copy-only-ui.json / copy-only-ui.mjs（同验收目录）。

## 2026-09-09 生产接入

用户授权更新 Blog、Docs、WWW 后，PAT 必要依赖同时部署：Identity sync-2、Account/Asset/Blog/Docs sync-1，0035 与 Docs 0019 已执行。独立固定候选构建，不是正式 GitHub Release。私网目录因 Identity 出站代理需添加 NO_PROXY 精确服务名，已修正。真实生产验收账号完成两站目录可达、创建、恢复/no-store、userinfo、权限拒绝与撤销401。发现 PG GetPATByHash 缺失记录错误映射，已有 PG 生命周期测试先红后绿；Identity 源码与线上均修复。临时 PAT 已全部撤销。详见 [Docs 部署记录](../../../../docs/flightdeck/work/2026-09-09-project-docs-publishing/deployment.md)。
