# 仓库说明

- 相邻 `../workspace` 存在且其 `repos.lock.yaml` 包含本仓库时，跨仓工作或任何本地进程生命周期操作前必须读取并
  遵守 `../workspace/docs/multi-project-development.md`。产品仓库会话把 Workspace 合同和 `.doctor/` 视为只读，
  生成状态只通过 Workspace CLI 操作；同仓并行写入必须使用独立 Git worktree，代理会话不拥有共享 Provider。
- 长期或可恢复工作通过 `flightdeck/deck.md` 对齐。
- 本仓库只承载可独立发布的公共基础模块，不引入具体产品或业务服务实现。
- Web 界面验收必须使用仓库内的 CLI Playwright 测试；不要依赖人工浏览或任何外部软件中的浏览器状态。
- 修改公共契约时，必须同时检查版本策略、兼容性、消费者迁移和发布说明。
