# Abuse

Abuse 是嵌入消费者数据库的公开 Go Module，负责实例本地外部输入准入：稳定 Action、原子多预算、
`allow / challenge / reject` 决策、结果 penalty、proof verification 和恢复治理。
它不是 HTTP limiter、Authorization、内容审核、账户锁定、bot score 或跨站风控服务。

## Language

**Action**：消费者声明的稳定外部操作，例如 `identity.password_login` 或 `blog.comment.create`。Action 不是 URL；
路由调整不得改变其身份。

**Attempt**：一次带稳定 Attempt ID、Action 和瞬时 Signals 的准入请求。

**Signal**：消费者从可信上下文提取并 canonicalize 的瞬时事实。常用 slot 是 Network、Actor 和 Target；
额外 slot 必须在 Definition 中声明。

**Subject Key**：Module 使用实例 secret、key version、Policy、slot 和 canonical Signal 派生的 HMAC 值。
它只在当前实例和用途内稳定，不是账户 ID 或跨站画像。

**Meter**：一个 Action 对一个 Signal slot 使用的独立预算。一次 Attempt 的全部 Meter 原子判定。

**Admission Meter**：`Admit` 返回 allow 时立即消费的预算。业务失败不会退款。

**Outcome Meter**：在 `Admit` 时建立 pending penalty、在 `Resolve` 时按 Outcome 确认或恢复的 exact sliding ledger。

**Admission**：可靠保存的 `allow`、`challenge` 或 `reject` 决策。它不是业务资格或授权判决。

**Receipt**：Attempt 的 durable idempotency 和 outcome lifecycle 记录。

**Outcome**：消费者从 Action Definition 的关闭集合中报告的结果。

**Challenge**：策略升级后要求的 provider-neutral server-side proof；通过 challenge 不等于可信用户或授权。

**Proof**：只在 verification 期间存在的短期 token。原值不得写入 Module state、error、audit 或普通日志。

**Governor**：管理员使用的 Action inspection、subject reset 和 retention pruning seam。

`Client Identity` 不作为聚合术语：Network、Actor 和认证 Target 必须保持独立维度。`Incident` 属于产品安全运营；
`Observation` 属于 Traffic/Audit 投影，均不是 Abuse 真值。

## Ownership

- Foundation 持有 Definition 编译、关闭的预算算法、整数转移、subject HMAC、原子多预算、decision 合成、
  receipt/outcome 状态机、Memory/PostgreSQL conformance、challenge port 和 Turnstile protocol Adapter。
- 消费者持有 Action 名称、阈值、trusted-proxy 解析、Signal canonicalization、HTTP 映射、业务资格、审核、
  账户状态和用户消息。
- Identity 证明 Subject 并拥有凭证、会话和账户恢复；Abuse 不读取 token 或设置永久账户锁。
- Authorization 决定角色、能力和申请；Abuse 只可在调用这些业务操作前 gate。
- Audit 可接收消费者选择的 coarse Action、Disposition 和 Attempt ID 投影；Abuse 不直接依赖 Audit。
- edge/shared HTTP limiter 继续处理粗粒度过载和 volumetric abuse；Abuse 处理已经到达应用的命名操作。
- PostgreSQL 真值位于每个独立消费者实例数据库，不存在共享 Store、远程 Adapter 或跨实例风险画像。

## Invariants

- Consumer 在启动时绑定已编译 Action；endpoint 不传 Policy ID、cost、capacity 或算法。
- required Signal 缺失是 invalid input，绝不归入共享 `unknown` subject。
- 原始 Network、Actor、Target、User-Agent 和 proof token 不持久化；只有 purpose-bound HMAC Subject Key 可落库。
- 相同 Attempt ID 与相同 fingerprint 是 replay；相同 ID 与不同 Action/Signals 是 conflict。
- 一次 Admit 的全部 Meter 要么消费、要么都不消费；锁按 canonical key order 获取。
- Admit/Resolve 使用 Abuse 自己的短事务，不接受 caller-owned transaction。
- business validation、insert 或 transaction rollback 不会撤销已经提交的 Admission Meter。
- provider 网络调用期间没有 Abuse 数据库事务；verification 后重新评估全部 Meter。
- valid proof 只能满足 challenge tier，不能越过 reject tier。
- Outcome Meter 的 pending 与 committed event 都占用容量；默认 Outcome 必须显式。
- 成功恢复只清理 Definition 指定且已经 committed 的 Target penalty，不清理其他并发 pending 或 Network budget。
- storage/provider operational error 与 policy decision 分离；任何 error 都不授权 caller 继续。
- receipt 保存 resolution plan，使旧 Definition revision 的 pending Attempt 仍可 Resolve。
- retention cleanup 缺席只增加存储，不改变准入正确性。

## Public seam

- `Compile(Definition)` 生成不可变 Catalog，校验 Actions、Signal requirements、Policies、算法、outcomes、challenge 和 limits。
- `Module.Action(ActionKey)` 在启动时返回 bound Action。
- `Action.Admit` 处理幂等、原子预算、challenge continuation 和 structured Admission。
- `Action.Resolve` 处理允许 Action 的关闭 Outcome 集合和 pending penalties。
- `Governor` 提供 Actions、Inspect、Reset 与 Prune；普通 endpoint 依赖不到治理能力。
- `ChallengeVerifier` 是 provider-neutral 外部 Port；fake 和 Turnstile 是 Adapter。
- Memory 是确定性 reference Adapter；PostgreSQL 是实例本地 durable Adapter；两者运行同一 conformance。

## Algorithms

- Token Bucket 是高频 Admission Meter 默认算法。
- Exact Sliding Window 用于低频严格 quota 和全部 Outcome Meter。
- Fixed Window 只在消费者显式选择时用于可解释业务 quota，不作为默认安全控制。
- 所有 cost、capacity 和转移使用整数语义；Adapter 不以浮点近似重定义行为。

## Policy evolution

- Definition Version 单调递增并绑定 canonical digest；相同 Version 的 digest 漂移和版本回退均拒绝。
- 改变算法 kind、Signal slot 或 admission/outcome mode 必须使用新 Policy ID。
- 兼容的 capacity/refill/window 调整保留已消费状态并 clamp，不自动 refill。
- 删除 Policy 只停止新消费；旧 state/receipt 按 retention 和保存的 resolution plan 收口。
- versioned profile helper 可以展开普通 Definition；已发布 profile 版本不得静默改变。

## Non-goals

跨实例/跨站风险图谱、共享远程 Store、设备指纹采集、IP/ASN reputation、TLS 指纹、行为生物特征、bot score、
内容黑名单、spam classifier、审核队列、永久账户锁、MFA、Authorization、HTTP middleware、DDoS mitigation 和
任意在线 policy DSL 均不属于 Abuse。
