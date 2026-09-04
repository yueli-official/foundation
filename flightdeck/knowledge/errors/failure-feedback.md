# Failure Feedback

前端把结构化 failure 解析成当前动作的反馈计划，不直接展示机器码、后端 message 或任意 thrown value。产品决定 inline、区域 Alert、Toast 或任务详情；Foundation Resolver 负责统一提取和安全降级。

## 反馈内容

用户反馈应同时回答：什么对象或阶段失败、用户现在能做什么、支持人员如何关联本次失败。

- 主文案来自产品 message key；缺失翻译时使用调用动作提供的安全本地化 fallback，例如“创建文档集失败，请稍后重试”。
- recovery key 必须对应产品中真实存在的动作；没有诊断页面时不得让用户“打开诊断信息”。
- code 和 traceId 只放在可复制的技术详情中，不作为标题或主要描述。
- 未结构化 transport failure 不得伪装成业务错误；显示动作级 fallback，并记录合同违规。

## 字段与区域

- 能映射到控件的 `violations[].pointer` 显示为字段 inline error，设置 `aria-invalid` 并通过 `aria-describedby` 关联说明。
- 未映射 violation 进入表单错误摘要，不丢弃。
- 影响整个集合或任务且需要持续阅读的失败使用区域 Alert。
- 没有稳定原地载体的跨区域失败才使用 Toast；同一失败不得同时重复显示。
- `role=alert` 只用于阻断且时间敏感的信息，普通状态更新使用较温和的 live/status 表面。

## 重试

`retryable` 不是“立刻自动重试”。客户端同时检查：请求是否幂等、是否持有 idempotency key、服务端是否已经开始执行、`Retry-After`、最大次数和用户意图。产生副作用的请求在结果不确定时不得自行重复创建；后台任务优先通过其稳定 ID 查询最终状态。

依据：[WAI 表单通知](https://www.w3.org/WAI/tutorials/forms/notifications/)、[WCAG 错误识别](https://www.w3.org/WAI/WCAG22/Understanding/error-identification)、[Stripe 幂等请求](https://docs.stripe.com/api/idempotent_requests)。

