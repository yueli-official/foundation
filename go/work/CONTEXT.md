# Work

Work 是嵌入消费者数据库的公开 Go Module，负责可靠后台工作、事务 Outbox、延迟/周期调度、租约执行与运维状态。
它不是跨站共享的任务服务，不拥有消费者的领域 payload、handler 或外部副作用。

## Language

**Job Kind**：消费者注册的稳定 handler key，例如 `asset.rebuild_derivatives`。

**Queue**：拥有独立并发上限的执行通道。

**Job**：持久命令 envelope。Foundation 只校验 JSON、大小、时间和注册关系，不解释 payload。

**Attempt**：一次获得租约的 handler 执行。Attempt 历史不可变，不因 replay 或 prune 以外的管理动作被覆盖。

**Lease**：由随机 token 标识的限时执行所有权。只有当前 token 可以 heartbeat、写 progress 或完成状态转换。

**Replay**：从终态 Job 创建一个新 Job，并以 `replay_of` 保留来源。它不重置原 Job。

**Dead Job**：达到最大尝试或永久失败的 `failed` Job。它仍可查询和 replay，不进入另一个隐藏存储。

## Ownership

- Foundation 持有状态机、Catalog、幂等 fingerprint、租约协议、backoff、Runner、Memory/PostgreSQL Adapter、迁移和 conformance。
- 消费者持有 Definition、InstanceKey、payload schema、handler、领域事务创建时机和副作用幂等。
- PostgreSQL 真值位于每个独立消费者实例自己的数据库；不存在多个站点共享一个 Work Store 的部署要求。
- HTTP/Admin 层持有鉴权、DTO 和操作审计。Work 不判断谁能 pause/cancel/replay。

## Invariants

- 同一 idempotency key 与相同规范化请求 replay；不同请求 conflict。
- Claim 按 priority、run_at、created_at 排序并原子取得一个 lease。
- Claim 增加 attempt 并创建 Attempt；同一 Job 同时只有一个有效 lease。
- Pause/cancel 会使旧 token 失效；旧 handler 不能覆盖新状态。
- 过期 lease 记录 `lease_expired`，在预算内 retry，预算耗尽进入 failed。
- handler 投递是 at least once；Work 不伪装 exactly once。
- 失败默认 retry；Permanent 与 RetryAfter 是显式分类。
- 周期 occurrence 使用确定性 idempotency key；进程重启不会复制 occurrence。
- PostgreSQL row 是真值；即使后续加入 NOTIFY，也只能作为 wake-up，轮询必须保持正确。
- Catalog/schema 不匹配时 fail closed。
- 迁移由消费者显式应用；应用启动不自动修改 schema。

## Public seam

- `Compile(Definition)` 生成不可变 Catalog。
- `Module` 提供 enqueue、query、management、replay 和 retention。
- `Backend` 增加 claim/heartbeat/progress/transition/schedule，是 Adapter 与 Runner 的 seam。
- `Runner` 负责 queue concurrency、handler timeout、heartbeat、retry 分类和 graceful shutdown。
- PostgreSQL `EnqueueTx` 接受 `*sql.Tx`，用于与消费者领域写入原子提交。
- `worktest.Run` 是所有 Adapter 的行为契约。

## Non-goals

跨站中央调度、workflow DAG、远程 worker protocol、Kafka/RabbitMQ/Redis transport、业务 payload schema、HTTP Admin API、
用户授权、外部副作用 exactly-once 和任意事件总线均不属于 Work。
