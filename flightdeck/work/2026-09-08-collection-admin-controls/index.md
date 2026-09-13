# 紧凑集合管理组件

## Goal

统一可复用的紧凑分页、网格密度、评论阅读布局和标题工具区，让产品通过数据 Adapter 接入。

## Status

Finished

## Current

2026-09-09 用户确认后推广：Gallery、Blog、Docs、BVideo 全部显式切到 columns，并传入已通过状态。共享 Toolbar 增加 ml-auto/max-w-full，修复 Blog 标题工具区右侧空白。规则已写入 compact-admin-collections.md 与 UI README。Blog/Docs 使用仅含评论组件差异的固定 UI 候选 `0.4.0-server.20260909.columns.1` 构建，线上 Web `server-20260909-columns-1` healthy；正式静态评论 chunk 与本地制品 SHA256 相同。没有正式发布 Foundation Release。证据 `E:/tmp/yueli-sites-sync-20260909/columns`；各产品真实后台验证在本地完成，线上无管理员会话，未冒称线上业务操作验收。

2026-09-09 增补本地试版：新增可选 `CommentModerationCollection layout="columns"` 与 `CommentModerationColumnsRow`。按作者正文、来源、状态、菜单分栏，窄屏重排，移除来源图片，审核放入菜单。仅 Gallery 显式使用，既有 table/compact 行为保持；未发布版本，未默认推广。共享类型检查、Gallery 类型检查/构建和四宽度真实 Playwright 通过，待用户确认。新增可选 prop 属于后续 minor 能力；发布前记录正式版本与消费者切换，当前 README 已标明试用边界。

共享分页、默认紧凑网格、标题工具插槽、筛选/排序草稿弹窗和紧凑评论已完成，规则已写入 [Knowledge](../../knowledge/frontend/compact-admin-collections.md)。用户确认 Gallery / Blog 后授权全站接入；产品布局与验收由各产品 Work 记录。标题工具插槽与空操作列修复已提交（`f54b00e`、`a2fb159`）；其余既有源码改动保留在工作树中，未正式发布 Foundation Release。2026-09-09 已将标题栏修复部署到 Blog、Docs、WWW、Account 的前端 `server-20260909-header-2`，Gallery 保持本地。

## Next

None

## Verification

- 35 个测试文件、106 项共享测试通过；后续仅筛选工具和布尔默认值修正的相关 10 项测试及类型检查通过。
- CLI Playwright 验证 390/1440 页面，网格另覆盖 3840，包含选中态、隐藏列表表头、首页/末页、修改筛选草稿后取消并重新打开恢复、树状视图切换；100 页折叠沿用已通过的 Blog 浏览器夹具验收。
- Blog、Docs、WWW、Account 的独立 admin.4 候选构建成功，Web 镜像更新并持续 healthy，正式公开页两种宽度与 Blog 共享分页复查通过。未宣称获得真实管理员权限进行所有线上业务操作。
- 其他消费者完成本地布局、类型和生产构建检查；没有首次上线。Workspace 原有共享 Provider、Blog、Gallery 保留，本轮额外验收组合已通过 CLI 停止。
- 证据：`E:/tmp/yueli-media-preset-20260908` 的各站浏览器 JSON、构建日志、截图、`admin-4-source-inputs.json` 与 `admin-4-web.sha256`。共享源码中后来增加的仅筛选选项由本地 Shortlink 消费，线上 admin.4 不需要该选项。

2026-09-08 用户修正分页：恢复紧凑模式首页与末页按钮，数量选项显示“20 条 / 页”，删除独立“每页”标签。分页回归与 Blog 分类/标签浏览器验收通过。

## 标题栏线上交付（2026-09-09）

- 仅替换各站既有候选 UI 包的 PageHeader/ManagePage，保留业务源码；四站类型检查、生产构建通过。后续空插槽补丁仅为 CSS，四站重新构建通过。
- CLI Playwright 验证 Gallery 三页在 390/1024/1440 的布局，以及空条件插槽；线上四站公开页在 390/1440 返回 200、无横向溢出或页面异常。Blog 后台验证共享分页及权限隐藏按钮时标题工具右侧间距为 0。
- 四站 Web 容器 healthy；Docs API 保持 parent-1，未变更后端、数据或其他站点。未声称验收四站所有管理员业务操作。
- [线上浏览器结果](references/header-production-result.json)、[空操作插槽验收](references/header-empty-actions-result.json)、[运行版本](references/header-runtime-final.txt)。构建、部署脚本与完整日志在 `E:/tmp/yueli-header-20260909`。

## Paste 验收补丁（2026-09-10）

CollectionPagination 的上游 ARIA 标签为英文，现通过原生插槽统一中文的首/前/页码/后/末页名称，保留原导航语义和紧凑尺寸。Paste 的桌面/手机100页折叠、页码跳转、每页数量以及28px尺寸已验证；Nuxt类型检查、生产构建与7项共享集合测试通过。本次为本地源码补丁，未发布新制品。
