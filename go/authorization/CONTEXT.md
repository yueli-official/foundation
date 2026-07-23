# Authorization

Authorization 是嵌入每个消费者实例的公开 Go Module。它以实例本地持久化为领域真值，解释 Subject、Access Layer、Role、
Capability、Scope、Grant、Group、Application、Invitation 和 Policy Revision；具体数据库和决策引擎位于 Adapter 后。

## Language

**Subject**：一次访问的主体。类型为 Anonymous、Guest、User 或 Service；Identity 负责证明身份，Authorization 不管理账户和凭证。

**Access Layer**：由 Subject 状态自动获得、不可授予的基础访问层。`visitor` 表示未登录访问，`authenticated` 表示持有效凭证的主体；
它们可以在 Policy 中绑定 Capability，但不是 Role。

**Role Definition**：策略内具有稳定 ID、key 与所属 Scope 的能力集合。消费者必须在代码中声明一个 Root Scope 的受保护管理员
Role；作者等内置 Role 也位于 Root，自定义 Role 则由有效管理者在其管理 Scope 内通过 Policy Draft 创建。Module 不预置业务角色。

**Capability**：消费者以代码注册的稳定、版本化动作键。管理端只能在 Catalog 允许的 Subject、Scope 和可委派边界内组合
Capability，不能创建任意字符串权限。Module 只保留 `authorization.*` 自身管理/工作流能力，不预置产品业务能力。

**Scope**：授权生效的层级节点。每个实例有一个 Root Scope；授予在本 Scope 及后代生效，结构必须符合消费者注册的 Scope Schema。

**Resource Relation**：产品领域持有的 owner、assignee 等事实。Authorization 通过 typed Resource Resolver 读取，不复制业务所有权。

**Role Grant**：Subject 或 Group 在 Scope 上获得 Role 的显式、可审计记录；具有来源、生效时间、到期时间和撤销生命周期。

**Group**：Authorization 持有的扁平 User/Service 集合。Group 属于 Scope，可被授予 Role；不允许嵌套 Group。

**Application**：一个 Subject 对一个 Role 和一个 Scope 的申请。批准会在同一事务中创建 Grant；终态不可改写。

**Invitation**：管理员或受委派者邀请已有 Subject 或经验证邮箱主体接受一个 Role/Scope Grant 的流程。

**Policy Revision**：Role/Access Layer 与 Capability 绑定、分配策略和自动规则的完整版本。Draft 经校验和影响预览后原子激活。

**Constraint**：消费者以代码注册、不可由管理员覆盖的安全规则，例如作者只能修改自己的文档、永久删除仅管理员可用。
Source Constraint 可显式选择内置 Role，或用 `AllNormalRoles` 覆盖现在及以后创建的全部非 protected Role，避免自定义 Role
绕过资源所有权规则。

**Decision**：带 allow/deny、原因、来源、Policy Revision 和 Decision ID 的授权结果；默认拒绝，多个 Role 只做 allow union。

## Public seam

- `Compile(Definition)` 校验 Capability Catalog、Scope Schema、受保护 Role、Constraint 与自动规则，返回不可变 Catalog。
- `Authorizer.Decide` 和 `Authorizer.BatchDecide` 处理单项/批量访问；`QueryPlanner.Plan` 返回 typed Query Constraint。
- `ResourceScopeRegistry` 供受信任的产品资源生命周期幂等登记 Scope；它不能创建 Grant，也不等价于用户可调用的
  `ScopeManager`。
- `RoleManager` 在 Policy Draft 中创建、更新或退役 Scope-owned Role；Role ID 和所属 Scope 不可变。
- 管理命令 Interface 执行全部授权写入；消费者不得直接写 Adapter 的私有表。
- Policy 管理创建 Draft、校验、预览、激活和回滚 Policy Revision；`PolicyReader.GetPolicySnapshot` 为管理界面提供
  transport-neutral 的有效 Role、Access Layer 和自动规则快照。
- Reconcile Interface 消费幂等 Identity 事件、补偿单个 Subject，并显式执行 dry-run/backfill。
- `WithRequestMetadata` 把调用链 correlation ID 带入管理与决策审计；Application 的显式幂等 key 支持安全重试。
- Casbin、PostgreSQL、HTTP、Identity event transport 和产品查询翻译器都位于公开 Interface 之后。

## Ownership

- 每个消费者实例拥有自己的 Authorization 数据、受保护管理员和策略；不存在平台超级管理员、共享授权库或跨实例传播。
- Identity 持有账户、凭证与稳定 Subject ID；其基础角色不能直接成为产品授权。
- 产品持有资源、业务关系和事务入口；Authorization 持有纯授权关系、工作流与审计。
- Foundation 持有普通 Go 领域核与 Adapter Interface，不依赖 Platform workspace alias、产品 DTO、内部域名或进程环境。
- Adapter 实现以 `authorization/authorizationtest` 的公共 conformance suite 验证相同行为，持久化实现另加事务与 schema 测试。

## Invariants

- 默认拒绝；没有管理员可配置的 deny，也没有 Role 继承。多 Role 通过 allow union 合成。
- 受保护管理员 Role 永远包含 `authorization.manage` 和消费者全部可管理 Capability，不能删除、降权或失去最后一个有效 Grant。
- Constraint 先于普通 Role allow 生效，管理员也不能绕过；系统紧急恢复必须走显式离线运维流程并产生审计。
- Role、Grant、Group、Application 和 Invitation 不能越过所属 Scope；委派者不能授予自己不可委派的能力或修改上级 Grant。
- Role ID 不变，显示名不参与判断；Scope 树无环；Group 不嵌套；未知 Capability、Scope Type、Trigger 或 Predicate 一律拒绝加载。
- Grant 到期或撤销后保留记录；Policy 激活原子切换；所有授权写操作追加审计。
- 相同 Subject 的 Application 幂等 key 只能代表同一命令；完全重放返回原申请，参数不同则冲突，且该语义跨进程重启保持。
- 决策投影可随时从实例本地领域表重建，不是恢复授权状态所需的真值。
