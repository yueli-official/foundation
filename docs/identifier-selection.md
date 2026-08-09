# Identifier 选型、迁移与发布说明

## 结论

Foundation 的 Go `go/identifier` 与 JavaScript/TypeScript `@yueli/identifier` 是站群唯一的普通 Identifier
生成入口。二者遵循同一份 `contracts/identifier` wire contract；UUID 分别由 `github.com/google/uuid` 与
`uuid` 实现。Foundation 只固定组织级 writer、严格解析、公开 Key Profile 和分配语义，不自行维护 UUID 位布局。

| 场景                                       | 选择                                              |
| ------------------------------------------ | ------------------------------------------------- |
| 数据库实体、事件、任务、投递               | Go `New()` / JS `newUUID()`，RFC UUIDv7           |
| 8 位公开 URL 地址                          | `CompactURLV1` + `Allocate()` / `allocateKey()`   |
| 6 位高密度公开路由                         | `ShortLocatorV1` + `Allocate()` / `allocateKey()` |
| 人工输入的非授权码                         | `HumanCodeV1` + `Allocate()` / `allocateKey()`    |
| 长期公开不透明引用                         | `OpaquePublicV1` + `Allocate()` / `allocateKey()` |
| 确定性跨系统映射                           | `Derive()` / `deriveUUID()`，UUIDv5               |
| 持有即授权的邀请/兑换/重置值               | capability/token Module，不属于 Identifier        |
| Trace、幂等、人类编号、handle/slug、cursor | 各自拥有的 Module                                 |

## 排除算法

- XID 暴露时间、机器、进程与计数器信息，且不提供密码学不可预测性；UUIDv7 已覆盖内部时序友好主键。
- Sqids 只可逆编码已有整数，不分配唯一值也不保密；若出现真实消费者，应建立独立 `integercodec` Module。
- ULID 与 UUIDv7 的用途重叠，但不能获得 PostgreSQL 原生 UUID 的同等互操作收益。

## 迁移策略

当前站群未上线且 seed 是唯一数据来源，因此直接统一 schema、seed、测试夹具和新写入：

- 产品不得再调用 `uuid.New*`、`crypto.randomUUID()` 生成持久业务 ID、自写 UUID 位布局或自选随机公开 ID alphabet。
- 数据库 UUID 列不再提供 `gen_random_uuid()` 或 `uuidv7()` 默认值；拥有用例在写入前从 Identifier Module 取值。
- Identity 只保留 UUIDv7 内部 ID、8 位稳定公开用户键和可变 handle，不保留重复的长 Public Key 与 Short ID。
- 不建设中心化 ID 网络服务；唯一 claim 与拥有行保持同一产品事务。

## 版本与发布

Identifier 是 Go Foundation 下一 minor 版本和 JS Foundation 下一 release bundle 的兼容新增。公开 Profile 的名称、
alphabet、长度、大小写和解析规则均属于持久化格式；修改必须发布新的 Profile 版本。正式消费者必须等待新的 Go tag
或 JS tarball 并升级依赖，不提交相对 `replace`、`link:` 或不存在的制品地址。本地站群工作区只通过 `.doctor/` overlay
把公共 import 临时解析到相邻 Foundation checkout。
