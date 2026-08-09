# Identifier

## Language

**Entity Identifier（实体标识）**:
站群内部持久实体、事件、任务或命令的不可变 UUIDv7。它改善索引局部性，但不表示严格业务顺序。
_Avoid_: 序列号、创建时间、授权凭证

**Public Locator（公开定位符）**:
可安全展示和记录、用于从公开路由定位对象的随机 Key。它不因“难猜”而获得授权含义。
_Avoid_: Secret、Bearer Token、Handle

**Candidate（候选值）**:
尚未通过拥有产品的唯一约束原子 claim 的随机 Key。
_Avoid_: 已分配 ID、全局唯一值

**Allocated Key（已分配 Key）**:
产品事务已在正确 namespace 下成功 claim 的 Public Locator。
_Avoid_: 单次随机生成结果

**Derived Identifier（派生标识）**:
由稳定 namespace 和版本化 canonical name 确定性产生的 UUIDv5。
_Avoid_: Hash、签名、匿名化秘密

## Invariants

- `New` 固定生成 RFC 9562 UUIDv7；改变 writer 版本是公共 Interface 变更，不能静默发生。
- UUIDv7 的时间部分不能替代 `created_at`、事务序列或跨节点全局顺序。
- Public Locator 使用密码学随机源和无模偏差编码，但不是 secret 或授权判断。
- Public Key Profile 是 Foundation 发布的不可变版本；产品不能自定义长度、alphabet 或随机源。
- 唯一性由产品拥有的具名数据库约束与原子 claim 决定，禁止先查询再插入。
- `Derive` 不规范化输入；canonical name 与 namespace 版本由拥有领域定义。
- 模块不访问数据库、网络、环境变量或产品配置。
