# 统一管理后台平台 context

## Language

**Admin Platform**：Foundation 提供的共享管理后台能力集合，由领域规则、HTTP 行为合同、UI 深模块与组合内核组成；不拥有任何产品业务数据。

**Product Definition**：产品代码中静态、强类型的后台组合声明，描述品牌、措辞、模块、导航与权限要求。它不是存储在数据库中的低代码页面定义。

**Admin Module**：围绕一个稳定后台能力形成的深模块，例如 Classification、Collection、Settings、Comments、Authorization、Submission Review。模块拥有公共交互和状态机，产品只提供 Adapter 与领域特有内容。

**Product Adapter**：产品自有的适配实现，把 Post、Image、Workflow、Product 等领域对象和 HTTP 路由接到公共模块接口。产品领域名不被强制重命名成 Resource。

**Product Extension**：确实没有跨产品稳定语义的专属页面或模块，例如 Docs 文档树、Licensing 激活治理、Nav 链接健康检查。

**Branding**：站点名称、Logo、主色、图标和少量措辞。运行时可变品牌数据与编译期模块组合分离。

## Decisions

- Foundation Classification 继续作为 Category、Facet、Facet Value、Tag 的规则权威；产品持有自己的数据库、事务和 HTTP Adapter。
- 公共后台模块优先共享行为与 UI，不建立跨站中央业务数据库。
- Product Definition 使用 TypeScript/Go 的 typed code，不用数据库 JSON 描述完整后台。
- 公共 Collection 合同统一查询、分页、排序、选择和批量行为，但保留 `/posts`、`/images`、`/products`、`/workflows` 等产品领域语言。
- Classification 的 HTTP 投影可以真正统一，因为其领域语义已经由 Foundation 统一。
- 运行时设置适合保存品牌、主题、站点文案、投稿状态等真正需要动态修改的值；模块拓扑、路由和权限声明属于版本化代码。
- 第一个消费者是 Yotta Hub；先验证组合内核和现有 taxonomy 页面接入，不改写当前未提交的销售/投稿工作。
- Yotta 试点证明 Classification 的第一个稳定共享 seam 是纯行为投影：树、祖先链过滤、分支分页、折叠、父级防环以及 Tag collection；HTTP、事务、revision、图片和产品字段继续留在 Adapter。
- 第二个消费者是 BVideo。它证明“多个 Facet + 每个 Facet 的递归 FacetValue 树”本身属于 Foundation Classification：共享层负责确定性树、path、同 Facet 隔离和父级防环；BVideo 保留 HTTP、事务、删除影响和排序写入。
- Product Definition 只描述静态、类型化后台拓扑；BVideo 的待审核视频数量等运行时数据由产品在 projection 上装饰，不进入 Definition。
- Yotta `filter-dimensions` 已通过薄 Adapter 消费 Foundation FacetValue 树和父级防环候选，没有新增公共 UI 状态机。后台 hierarchy/parent semantics 属于 Foundation；运行时 `filterRows` 的 `available/leaf` 计算继续属于 Yotta。
- Facet assignment policy 的 canonical 字段是 `minValues`、`maxValues`、`leafOnly`、`maxDepth`，Foundation Go Classification 已负责范围验证和实际 classify 约束。
- Yotta 的 shared/system/local 来源、`authorVisible`、`discoveryVisible`、`allowedValueIds`、binding revision/HTTP transport，以及“作者不可见时不能要求必填”的规则继续是 Product Extension。只有一个产品拥有这组行为，当前不抽通用 binding 模块。
- Foundation `classification/adminapi` 是 canonical Classification 的 framework-neutral HTTP/JSON vocabulary；它拥有资源 DTO、policy、mutation 和 revision wire semantics，但不拥有 Router、鉴权、持久化或事务。
- Canonical managed resource 使用 `id/slug/name/status/revision`，层级资源增加 `parentId`，FacetValue 始终保留 owner `facetId`，共享后台说明使用 `description`；`editorialPosition/replacementId` 保持可选治理字段。小集合返回 `{items:[...]}`，保存返回保存后的 Resource DTO，无正文删除返回 204。
- 产品展示/统计差异用 typed composition 增加字段，例如 Yotta Category 的 `icon/imageMediaKey/contentCount`；不提供通用 `extensions` JSON bag。多 scope 产品可以在 route 中加入 scope selector，但 selector 不进入 canonical DTO。
- Yotta Registry 在 Foundation 新 package 尚未正式 release 前只保留一个同形的内部 wire declaration；正式 release 后直接切换 import。禁止用 Go `replace` 或跨仓源码路径把未发布 Foundation 代码接进 Registry。
- Foundation Collection 的共享边界已经由 Blog、Gallery、BVideo 和 Yotta 作品管理共同验证：公共层负责 loading/error/empty、选择、批量区、列表/网格容器、分页与排序表头等稳定集合交互；产品仍可以通过 `externalControls` 在外部保留动态筛选编辑器和领域动作。Yotta 的多维 Taxonomy 筛选不需要为了“统一”被压进单值 `CollectionControl`，审核、上下架与运营标记继续是产品 Adapter/Extension。
- Foundation Settings 也已由 Shop、Resource、Gallery、Blog 与 Yotta 验证为成熟深模块：公共层拥有 JSON-safe baseline/dirty、capture/discard、Router/beforeunload 离开保护、响应式分区导航、SettingSection 与安全区保存 Dock。产品只负责字段、保存 API、权限与领域文案；Yotta 不再自己维护离开保护和顶部保存按钮。
- Foundation Comments 现已由 Yotta 作为真实产品 Adapter 验证：公共层拥有评论审核集合的 loading/error/empty、响应式行、分页、选择与批量操作 anatomy；Yotta 保留评分、作品来源、隐藏/恢复 mutation、reason/revision 等产品语义。Comment selection 的可选 `isSelectable(id)` 属于通用集合约束，可让某些可见记录不参与批量操作，不应扩成产品状态枚举或 metadata bag。
- Yotta `admin/users.vue` 与 `ReportManager.vue` 已进一步证明 Users/Reports 不需要新的万能 Admin Module：两者都只是 Foundation Collection + 产品行内容/筛选/mutation。`audit.vue` 已经通过薄 `HubCollection` 消费同一 seam。
- Authorization 第二消费者已经完成。Yotta 与 Blog 的申请审核都直接组合 Foundation `AuthorizationApplication` 与 `CollectionPanel`；Blog 的授权用户集合进一步组合 `AuthorizationUser`、`AuthorizationGrantBadge` 与 Collection。共享层不拥有 identity lookup、review/grant/revoke mutation 或角色名称映射。
- Hub 与 Blog 的角色策略编辑只有视觉相似：Hub 在首次修改时自动创建 draft，并把角色是否可申请建模为 assignment source；Blog 要求显式进入 draft，还拥有“注册用户自动成为作者”自动规则和 invitation source。真实第二消费者已经证明 policy lifecycle 不同，因此当前不提取通用 `AuthorizationConsole`。只有后续消费者出现相同策略状态机时才重新评估。
- Blog 消费 Foundation UI 使用独立打包的 tarball 候选包，不引入 Foundation 源码路径。当前 Foundation `test:pack`、Blog typecheck、产品测试与 production build 已通过；Blog 权限页 E2E 已改为公共 Collection 选择区和授权原语断言，Windows 更新后已通过 Workspace Isolated 恢复真实组合并完成权限页登录、选择与响应式验收；正式 Release 消费仍需单独验证。
- “Submission Review” 当前不提取成万能模块。Gallery 的处理/安全/批量审核、BVideo 的审核中编辑与仅保存、Yotta 的付费不可变版本核对/发布/退回存在不同状态机；它们继续组合 Collection、Feedback、Editor 等原语，直到出现真正相同的 review workflow。
