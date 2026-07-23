# Audit

Audit 保存每个独立消费者实例中具有安全、治理或问责意义的动作证据，使管理员能够回答谁在何时对什么做了什么，
结果如何，以及该动作与哪次请求或领域变更相关。

## Language

**Audit Event**:
对一次已尝试、被拒绝或已完成 Action 的不可改写证据，包含 Actor、Target、Outcome、Correlation 和最小 Evidence。
_Avoid_: Log Entry, Activity, History Row

**Actor**:
发起 Action 的 User、Guest、Service、System 或 Anonymous Subject；Actor 是稳定身份引用，不是显示名称或联系信息。
_Avoid_: Operator, Username, Email

**Action**:
消费者声明的、带版本且命名空间化的领域意图，例如授权 Grant 创建、站点资料发布或数据导出。
_Avoid_: Event Type, Operation String

**Target**:
Action 主要作用的稳定领域对象引用；批量动作引用批次或集合目标，而不是复制全部对象内容。
_Avoid_: Payload, Resource Snapshot

**Outcome**:
Action 的业务结果，区分 Succeeded、Denied 与 Failed；Outcome 不是日志严重级别，也不是 Target 的生命周期状态。
_Avoid_: Status, Severity

**Evidence**:
由 Action 合同允许的最小结构化元数据、字段名和摘要，用于解释 Outcome，但不复制业务 payload、凭据或不必要的个人信息。
_Avoid_: Detail, Arbitrary Metadata, Payload

**Correlation**:
把 Audit Event 与请求、trace、因果事件或批次关联的引用集合；Correlation 不决定事件保留，也不承载身份或敏感数据。
_Avoid_: Context Blob, Trace Payload

**Retention Class**:
Action 合同指定的保留类别，用于决定 Audit Event 的最短保留与到期处理，而不是由调用方逐事件选择任意期限。
_Avoid_: TTL, Expiry Flag

**Audit Journal**:
一个消费者实例持有的、按接收顺序追加的 Audit Event 集合；它是审计查询与导出的本地真值，不是业务状态或遥测后端。
_Avoid_: Central Log Service, Event Store
