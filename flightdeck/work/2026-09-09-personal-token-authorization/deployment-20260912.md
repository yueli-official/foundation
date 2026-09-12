# 2026-09-12 生产更新

所属仓库：foundation。状态：本轮服务器更新完成；不表示本 Work 的正式 SDK 发布等其他门禁完成。

- 固定候选源码 HEAD：`3ad8f48f942ddf5b1e7370b0d9b16fcc9bb999b2`；未提交改动通过逐文件 SHA256 与补丁归档，包括 Blog/WWW PAT 和 Foundation 编辑器按钮居中修复。
- 本批九个服务：Identity、Account Web、Asset、Blog API/Web、Docs API/Web、WWW API/Web，全部 `server-20260912-1` 且 healthy，宿主机端口仍只监听回环地址。
- WWW 新增生产 PAT Site ID 与 Identity 地址；Identity 追加 WWW 权限目录及 NO_PROXY 主机；Asset 追加 WWW 媒体授权回查。现有目录、密钥、登录 seal、数据库、媒体配置保留。
- 五个数据库与配置已备份到 `/projects/yuelili.com/backups/services-20260912`，pg_restore --list 验证通过；没有新增迁移，生产迁移原始字节保持一致。其他容器镜像未改变。
- 制品与运行证据：`E:/tmp/yueli-services-20260912`；服务器对应 `/projects/yuelili.com/.deploy/services-20260912`。source-manifest.json、packages.json、release-manifest.json、bundle.sha256、verification.json、config-verification.json 可审计源码/制品/镜像与配置。

## 验证

六个 Go 模块全量测试/vet、五个 Linux API 和附属命令构建通过。Docs、Blog、WWW、Account 的独立私有 tgz 安装/锁定、单测、typecheck、Nitro build 通过；Windows 输出转为内部相对 symlink，无 .node 二进制。

CLI Playwright 实际访问 19 组公开/账户页面，覆盖 1440/390 宽度、两种主题与 GlyphShift 六篇文档；0 pageerror、0 可见破图，抽查截图渲染正常。Docs/Blog/WWW 登录回跳与每站三次刷新正常。WWW 导航点击、Account 令牌表单打开/取消通过。

真实普通账户的权限目录无 unavailableSites；临时 PAT 创建、资料读取、非 WWW audience 拒绝、无效 PAT 不回退 Cookie、撤销后资料接口 401 均通过，临时令牌已全部撤销。原整体验收脚本误将无 WWW audience 预期为 403；共享 verifier 合同明确为 401，修正断言后仅重跑 PAT 生命周期通过。保留原失败 receipt，汇总见 delivery-summary.json。

## 已知限制与同步结论

- 生产验收使用已有普通账户，没有冒充管理员、改角色或改业务正文；管理员完整投稿沿用此前真实本地专项验收，不能称为生产管理员写入验收。
- GlyphShift v0.4.0 与 v0.4.1 的 docs.zip 都为 SHA256 `91f62b5c707d4a0cb15b41499a63e9d4b49e568cd5e277a96886c722de79c83e`。下载逐文件相同，线上六篇已发布正文逐篇与最新包一致。因此 skipped 是附件没有变化；本轮没有不同的新包可用于更新写入验收。
- 升级前曾复现 Docs/WWW 登录回跳临时错误；升级后本轮登录与刷新正常，但没有据此宣称所有间歇性 500 永久消失。
- Docs `/robots.txt` 升级前后均 500：discovery source_order_violation。只读生产查询返回 4086 条、无重复 Path，但 PostgreSQL 默认排序不等于 Go 字符串严格递增（例如带 ?locale 与子路径顺序）。此次按用户优先部署最新服务，未修改该 DAO 排序实现；后续修复应覆盖游标比较、排序与非 ASCII 路径。
- 固定私有 server 候选不是 GitHub 正式 SDK Release；没有 Git commit/push/tag，也未停止本地开发组合。
