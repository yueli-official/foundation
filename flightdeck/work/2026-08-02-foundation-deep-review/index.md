# 2026-08-02 Foundation 深度 Review

- Status: Finished
- Current: 仓库级 Review、P0/P1 加固、接入资料和全量验收均已完成；跨服务迁移与外部仓库设置已记录为后续事项。
- Next: None

## 目标

从公共基础库维护者和消费者两个视角，对 Foundation 的 Go/JS 模块、契约、版本发布、安全默认值、兼容策略及接入资料做一次仓库级深度 Review；修复确认的高优先级问题，并留下可复用结论。

## 边界

- Foundation 只提供可独立发布的公共基础模块，不接收产品或服务实现。
- 已完成的单模块审计只复用结论，不无差别重做。
- 现有 `js/apps/ui-lab/app/app.vue` 与 `js/conformance/ui/app/app.vue` 修改不属于本工作，必须保留且不得纳入提交。
- 不执行推送、打标签或发布。

## 工作文件

- [上下文](context.md)
- [计划](plan.md)
- [结论与后续事项](findings.md)
- [官方实践调研](references/market-and-release-research.md)

## 完成结果

- 对 Go/JS 模块、发布物、标签、契约、安全默认值和消费者接入完成带证据的仓库级盘点。
- 新增严格 access token verifier 与算法绑定 JWKS 校验，保留通用 JWT verifier 供 step-up/自定义 token 使用。
- 清除全部可达 Go 漏洞与已知生产依赖漏洞，发布工作流增加固定 Action SHA、验证、完整包集合、清单、校验和与来源证明。
- 新增消费者接入和发布策略文档；六个公开 JS 包的版本与发布集合现在可机器校验。
- 修复公共 Admin/Collection/Settings 组件的 landmark、导航标签、分页标签及滚动批量操作问题，并通过 CLI Playwright。

## 完成标准

- Go/JS 当前基线与失败项有可复现记录。
- 公共模块、版本、发布物、兼容边界与消费者依赖关系均被盘点。
- 结论区分事实、风险和建议，并标注优先级。
- 确认的高优先级问题已修复、测试并形成独立提交。
- 接入和维护文档足以让后续站点/App 避免重复调研。
