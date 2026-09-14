# 统一管理后台平台

**Status:** Open

## Goal

把站群中已经重复出现的后台壳、集合管理、分类体系、设置、审核等能力收敛为 Foundation 可独立发布的深模块；产品以 typed Product Definition 组合公共模块，并通过产品 Adapter 接入自己的领域数据和 HTTP 合同。第一消费者使用 Yotta Hub 验证组合方式，避免形成万能低代码后台或跨产品共享数据库。

## Current

2026-09-14 23:07（北京时间）：Foundation Go `go/v0.5.0` / JS `js-v0.8.0` 已正式发布，CI 与 JS Release 均成功；Yotta Registry 与 Hub 已切换正式依赖、完成 review/测试并部署 https://yotta.yuelili.com 。完整结果见 [发布验收](release-result.md)。下方候选及等待状态是历史记录。BVideo/Blog 正式依赖切换与新产品生成器不属于本次 Yotta 服务器交付，仍待后续处理。

2026-09-14 后续用户已授权 review/测试通过后提交并发布服务器。规范与需求双路 review 已完成；Foundation 子导航 search:false 投影遗漏已通过红绿测试修复，其他发布前修复由 Yotta 产品仓承担。正在发布 Go 0.5.0 / JS bundle 0.8.0 并切换正式依赖。以下为已验证候选事实。

2026-09-14 Windows 更新后恢复完成。后台组合、Classification wire、Yotta/BVideo/Blog 消费者改动均保留；本地发布候选已收口，尚未提交或发布。

Foundation `@yueli/ui` 已从 0.4.0 升到 0.5.0，并修复新增文件格式。目标为 Go `go/v0.5.0` 与 JS bundle `js-v0.8.0`，完整范围、消费者切换与证据见[发布候选](release-candidate.md)。Go 候选也包含 0.4.1 后已提交的个人令牌授权和 HTTP contract 投影变化，不只含分类合同。

本轮通过：Go 全量 race、vet、tidy diff、govulncheck；JS frozen install、完整 format/lint/typecheck/unit/build、七包 dry-run、UI tarball 独立消费者 typecheck/build；OSV 扫描 1193 packages 无问题。CLI Playwright：Foundation UI 12 passed / 2 个专用消费者测试按配置 skipped，HTTP 3 passed；Blog 权限管理 1 passed，覆盖真实登录、选择/取消选择、权限页、390/768/1280px。此前“Blog 缺少凭据未执行”的状态已消除。

Blog 已通过 Workspace Isolated 正式流程恢复，Session `20260914T141923Z-28820`，入口 `http://blog.dev.yuelili.test:3002`，Account `http://account-blog.dev.yuelili.test:3000`。未停止其他组合。候选七包与 SHA256SUMS 在 Foundation `.cache/admin-platform-release/`；未生成带虚假 commit 的正式发布 manifest。

## Next

Yotta 本次服务器交付已完成。后续在相应产品任务中让 BVideo/Blog 锁定正式 Release tarball 并验收，不重复发布已存在的 Foundation tags。

新产品创建的后续旅程已在候选文档列清：独立仓库 → 能力接入 → 本地组合 → 首位管理员/首个业务对象 → 交付；现有 Workspace bootstrap 仅物化锁定仓库，尚未实现新产品生成器。Authorization/Submission Review 保持已验证的共享原语与产品状态机边界。

## References

- [架构设计](../../../docs/admin-platform.md)
- [领域上下文](context.md)
- [实施计划](plan.md)
