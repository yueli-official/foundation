# Traffic

Traffic 是嵌入消费者数据库的公开 Go Module，负责 typed resource view 的可信计数、事件幂等、按日访客去重和聚合查询。
它不是通用埋点平台，也不拥有产品资源、HTTP 路由、用户身份或部署级隐私政策。

## Language

**Observation**：消费者已经完成资源可见性检查和流量分类后提交的一次资源访问观察。

**Event ID**：由发送方在投递前生成的稳定幂等键。相同 ID 与相同 payload 是 replay；相同 ID 与不同 payload 是 conflict。

**Resource**：消费者注册的 `kind + id`。Module 不外键关联、读取或解释产品资源表。

**Visit Class**：`unknown`、`human`、`bot`、`internal`。Catalog 决定哪些 class 进入聚合；默认仅 unknown/human。

**Visitor Token**：由 Adapter 使用实例 secret、Catalog digest、本地日期和消费者临时 seed 派生的 HMAC 值。它只用于当天去重，
不是账户 ID、cookie、跨日 people ID，也不能反推出 IP/User-Agent。

**View**：每个 counted Observation 增加一次。访客在同一天重复访问同一资源可以产生多个 view。

**Unique Visitor Day**：某个 visitor token 在一个本地日期、一个 scope 的首次出现。范围查询求和每日 unique，因此字段明确命名为
`UniqueVisitorDays`，不承诺跨日去重人数。

**Scope**：查询与去重仅有 instance 和 resource 两层。产品权限 Scope、组织层级和站点之间关系不属于本 Module。

**Baseline**：从旧计数器导入的 all-time views。Baseline 不生成 daily 或 historical unique 数据。

## Ownership

- Foundation 持有领域语义、校验、幂等/去重协议、聚合接口、Memory reference Adapter、PostgreSQL Adapter 与 conformance。
- 消费者持有 ResourceKinds、实例 key、IANA time zone、请求资格判断、bot/internal 分类、consent/DNT/GPC 策略和 product read projection。
- Identity 只提供已验证 subject。Traffic 不读取 token、账户或全局角色。
- HTTP 层持有可信代理解析、EventID/occurredAt transport 和重试策略。核心包不依赖 HTTP framework。
- PostgreSQL 真值位于每个独立部署的消费者数据库；不存在共享 Store、远程 Adapter 或跨实例汇总。

## Invariants

- Record/RecordBatch 先完整校验再原子提交；Batch 保持输入顺序且不会部分写入。
- Event receipt 与 visitor marker 是两个独立机制：前者防投递重放，后者防每日 unique 重复。
- 相同 EventID 的 fingerprint 包含资源、时间、class 和 visitor token；变更任一字段都冲突。
- 同一 visitor 同一天跨多个资源访问，instance unique 只增加一次，各 resource unique 分别增加一次。
- 时间以 Catalog 的不可变 IANA time zone 切分日期；`DateRange` 是 `[From, To)`。
- bot/internal 等 dropped Observation 仍保存 receipt，因此重放仍稳定，但不修改 aggregate。
- 原始 IP、User-Agent、URL、query、subject 和 email 不进入 Module persistence。
- Prune 只删除过期 receipt/marker；不删除 totals/daily。
- ForgetResource 不回减 instance aggregate，避免破坏访问过多个资源的 visitor 并集语义。
- PostgreSQL instance 的 schema version、Catalog version/digest/time zone 不匹配时拒绝启动。
- InitialBaselines 与首次 instance 创建在同一事务中；启动中断不会留下半导入状态。

## Public seam

- `Compile(Definition)` 生成不可变 `Catalog`，约束 ResourceKind、counted class、时区与 limits。
- `Recorder` 接受单项/原子批量 Observation 并返回当前 instance/resource totals。
- `VisitorTokenizer` 把消费者短暂持有的 seed 立即变成 daily opaque token。
- `Reader` 提供 Summary、补零 Series、Top 和批量 Totals，不暴露 SQL 表。
- `Importer` 提供显式历史 all-time view 回填。
- `Maintainer` 提供 retention pruning 与资源级隐私删除。
- `Module` 组合以上小接口；消费者可依赖所需的窄接口进行测试。
- `traffictest.Run` 是 Adapter 的可复用行为契约。

## Non-goals

Session、funnel、conversion、任意 custom event、referrer、UTM/广告归因、跨日 people、实时流处理、跨站统计、Dashboard UI、
身份识别、认证授权和产品资源生命周期均不属于 Traffic。
