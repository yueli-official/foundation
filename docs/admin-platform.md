# Yueli Admin Platform

## 目标

Yueli Admin Platform 把站群后台中稳定重复的行为收敛到 Foundation，让产品只维护领域数据、Adapter 和真正独有的页面。它不是站群 CMS，也不是一套用 JSON 动态生成任意业务页面的低代码系统。

现有站点已经覆盖内容、媒体、资源目录、交易、工具、软件发行等形态。跨这些产品稳定重复的部分包括：后台壳、集合查询与批量操作、Classification、Settings、Comments、Authorization、Asset 管理、审核队列以及一致的反馈行为。

## 分层

```text
Foundation domain rules
  Classification / Authorization / SiteProfile / HTTP Result / ...
                    ↓
Shared admin behavior contracts
  collection query / classification governance / settings / review / lifecycle
                    ↓
Shared admin UI modules
  AdminConsole / Collection / Classification / Settings / Comments / Authorization / Review
                    ↓
Admin composition kernel
  Product Definition → visible modules → navigation/search/current context
                    ↓
Product adapters and extensions
  Blog / Gallery / Resource / Shop / Yotta / ...
```

每一层都只暴露调用方必须知道的接口。删除公共模块后，如果相同状态机和交互会重新散落到多个产品中，说明这个模块有足够深度；如果公共层只是转发一个产品函数，则不应提取。

## Product Definition

Product Definition 是版本化代码，负责描述一个产品后台“由什么组成”，不描述业务页面内部的所有字段。

```ts
export const admin = defineAdminProduct({
  id: "yotta-hub",
  brand: { label: "Yotta", icon: "i-tabler-route", to: "/" },
  vocabulary: {
    resource: "作品",
    submission: "投稿",
    category: "分类",
    tag: "标签",
    facet: "筛选维度",
  },
  modules: [
    adminModule({
      id: "dashboard",
      label: "控制台",
      to: "/admin",
      icon: "i-tabler-dashboard",
      match: "exact",
      access: { any: ["yotta.workflows.manage"] },
    }),
    adminModule({
      id: "works",
      label: "作品",
      to: "/admin/workflows",
      icon: "i-tabler-route",
      access: { any: ["yotta.workflows.manage"] },
    }),
    adminModule({
      id: "classification",
      label: "分类",
      to: "/admin/categories",
      icon: "i-tabler-category",
      access: { any: ["yotta.settings.manage"] },
    }),
  ],
});
```

组合内核只负责：

- 校验产品与模块 ID、路由等静态不变量；
- 根据 capability 投影可见模块；
- 根据当前路由计算活动模块；
- 生成 `AdminConsoleLayout` 所需导航与命令搜索项；
- 提供稳定 vocabulary lookup。

它不负责 fetch、CRUD、表单字段、路由注册、数据库 schema 或权限决策本身。

## Admin Module

完整公共模块由四个部分组成：

1. **Domain/behavior contract**：稳定术语、不变量和动作，例如 Category reparent/delete、Submission review、Collection selection。
2. **Transport projection**：推荐的 HTTP DTO、查询参数和失败语义。产品可通过 Adapter 过渡，最终公共语义一致的领域应共享合同。
3. **UI module**：负责已经跨产品验证的布局、状态机、加载/错误/空态、批量操作、编辑 Overlay 与反馈。
4. **Product Adapter**：实现当前产品的请求、权限和领域对象映射。

模块不直接知道产品站点 URL、数据库表或产品 capability 名称。

## HTTP 合同策略

### 真正统一的领域

Classification 已经由 Foundation 统一领域语义，因此 HTTP/JSON 投影也统一。正式 wire vocabulary 位于 Go package `classification/adminapi`；它只定义 DTO、policy 与 mutation shape，不拥有 Router、鉴权、数据库或事务。

下面是选择一个 Classification scope 后的推荐相对资源面：

```text
GET    /admin/classification
GET    /admin/classification/categories
PUT    /admin/classification/categories/{id}
DELETE /admin/classification/categories/{id}
GET    /admin/classification/facets
PUT    /admin/classification/facets/{id}
GET    /admin/classification/facets/{facetId}/values
PUT    /admin/classification/facets/{facetId}/values/{id}
GET    /admin/classification/tags
PUT    /admin/classification/tags/{id}
```

多 scope 产品可以在 `classification` 与资源集合之间增加产品自己的 scope selector，例如 Yotta 使用 `/v1/admin/classification/{kind}/categories`；selector 不是 canonical DTO 字段。

Canonical DTO 使用 Foundation 语言：`id/slug/name/status/parentId/revision/editorialPosition/replacementId`，共享后台说明使用 `description`，Facet Value 保持 owner `facetId`，Tag 保持扁平。小集合返回 `{items:[...]}`；保存成功直接返回保存后的 Resource DTO；无正文删除返回 204。revision 属于 wire/persistence 并发语义，不需要污染 Classification domain entity。

产品可以通过强类型组合增加自己的展示/统计字段，例如 Yotta Category 的 `icon/imageMediaKey/contentCount`。公共 DTO 不提供通用 `extensions` JSON bag；`shared/system/local`、visibility、allowed subtree、binding revision 等 Yotta 状态也不进入 canonical Facet。

### 只统一行为形状的领域

Post、Image、Workflow、Product、Link 等继续保留自己的资源名与 endpoint。它们只对 Collection 行为保持一致：

- 搜索与筛选始终作用于完整远程集合；
- 明确的 page/cursor、pageSize、sort、direction；
- revision/ETag 负责并发编辑；
- 批量操作明确逐项失败语义；
- 加载、空结果、请求失败是不同状态；
- 选择模型由服务端集合语义决定，不假装对未加载行做客户端全选。

## UI 组合规则

公共 UI 的首要 seam 是现有 `YAdminConsoleLayout`、`YManagePage`、Collection、Settings、Comments、Authorization 等模块。Product Definition 不生成另一套 Shell。

普通后台页只能优先落在这些原型：Dashboard、Collection、Settings、Authorization/Governance、Editor。新的完整领域状态机在第二/第三个真实消费者出现时继续提取成 Foundation Module。

产品允许：

- 改品牌名、Logo、主色、图标；
- 改稳定名词，例如“资源”显示成“工作流”；
- 提供字段、列、动作和 Adapter；
- 添加真正产品专属 Module。

产品不应：

- 复制 Sidebar/Topbar/PageHeader/Toast；
- 为同义 Collection 重写搜索、分页、选择状态机；
- 用数十个布尔开关把一个共享组件变成所有页面的万能组件；
- 把完整后台布局序列化到数据库让运行时任意改变。

## Branding 与 vocabulary

品牌视觉继续使用 `createUiPreset` 和 SiteProfile。Product Definition 中保存的是代码需要的稳定 identity/default，运行时 SiteProfile 可以覆盖允许动态修改的品牌值。

Vocabulary 只改用户可见术语，不改变领域类型：

```ts
adminTerm(product, "resource", "资源"); // Yotta => 作品
```

错误码、HTTP 字段和数据库列不得因为 UI 措辞改变。

## 扩展与维护

新增公共能力前满足至少一个条件：

- 已有两个以上真实消费者出现相同状态机；
- 一个基础领域（例如 Classification、Authorization、Asset）本身已经是平台公共领域；
- 删除该模块会让重要复杂度在多个调用方重新出现。

模块接口优先稳定，产品差异放 Adapter 或 slot；当 slot/flag 数量开始描述完整业务流程时，应拆成产品 Extension。

公共模块必须拥有行为测试；消费者只验证 Adapter、权限、路由和关键集成，不复制公共模块全部测试。

## Yotta 试点

Yotta Hub 已经使用 Foundation AdminConsole、Collection、Settings、Comments 和主题，因此首轮没有重做视觉。2026-09-14 的试点已经验证两层公共 seam：

1. `admin.vue` 的硬编码导航已迁入 typed Product Definition。Foundation 负责 capability 投影、active route、命令搜索、品牌默认值和 vocabulary；Yotta 仍负责 capability 的真实含义。
2. Classification 首先抽取纯行为投影，而没有把 Yotta 页面或 Registry 请求搬进 Foundation。公共层现在负责分类树构造、搜索时保留祖先链、整分支分页、折叠、父分类防环候选，以及标签搜索/状态/排序/分页；随后 `classification/adminapi` 固化 canonical HTTP/JSON vocabulary。Yotta Registry 用薄 wire Adapter 把持久层 `key/parentKey/active/position` 映射成 `id/parentId/status/editorialPosition`，Hub 分类/标签后台直接消费 canonical wire；图标、图片与内容计数仍是 Yotta 的 typed extension。
3. Yotta `categories.vue`、`tags.vue` 与 `filter-dimensions.vue` 都已消费公共 Classification 投影。`filter-dimensions.vue` 只把本地/公共维度的 `FilterValue` 通过薄 Adapter 映射为 canonical FacetValue，再复用 Foundation 的递归树与循环安全父级候选；Registry 数据合同和 Yotta binding 仍留在产品层。
4. Foundation 全量 UI 测试、类型检查、真实 tarball consumer build，以及 Yotta typecheck/build 和 CLI Playwright 管理后台旅程均通过。浏览器试点覆盖 projected navigation、分类父链搜索、标签排序/保存、FacetValue 树层级、父级防环、Yotta binding、系统筛选只读视图、390px 布局和运行时错误。

## Yotta Collection 消费验证

Yotta 的作品管理原本自己维护搜索后的远程集合状态、空态/错误态、分页、页内选择、批量操作、表格、卡片和响应式布局。Blog、Gallery 与 BVideo 已经证明 Foundation Collection 能稳定承担这些交互，因此 Yotta 不再复制这层集合壳。

2026-09-14 的迁移保持 Yotta 领域接口和筛选能力不变：

1. `WorkflowManager` 使用 `CollectionPanel` 与 `CollectionSortHeader` 负责 loading/error/empty、页内选择、批量操作区、列表/网格容器和统一分页；上下架、运营标记、创作者编辑与 Registry 请求仍是 Yotta 行为。
2. Yotta 的多维 Taxonomy 筛选继续使用产品自己的筛选编辑器，并通过 `externalControls` 与公共 Collection 组合。共享 Collection 不要求所有产品筛选都退化成单值 `CollectionControl`；只有稳定重复的集合状态机需要统一。
3. Hub `pnpm typecheck`、production build、Go race/vet 与正式 contracts 均通过。Workspace 重启真实 Hub Web 后，CLI Playwright 验证 `/admin/workflows` 的桌面列表/卡片切换、分页、批量选择和 390px 无横向溢出，同时原有市场、Classification 与创作者旅程继续通过。

这进一步确认 Collection 的平台 seam 是“稳定集合行为 + UI anatomy”，产品只提供远程查询、领域行内容、筛选编辑器和业务动作。Product Definition 不需要承载页面字段或筛选 schema。

后续盘点又补齐了三类真实管理集合：

1. `admin/users.vue` 从手写 loading/error/empty、表格/卡片和分页迁到 `CollectionPanel`。用户资料、优质作者状态、角色授权与对应 mutation 仍属于 Yotta。
2. `ReportManager.vue` 的管理员举报和用户反馈共用同一个 `CollectionPanel` 集合壳；Yotta 继续拥有举报原因、处理状态和处理弹窗。远程分页没有被搬进 Foundation。
3. `audit.vue` 已经通过 Yotta 的薄 `HubCollection` 包装消费 Foundation Collection，因此不再为了“统一”重写一遍。

真实 Workspace Web 验收覆盖了用户列表/卡片切换与详情、举报列表与处理弹窗，以及 390px 无横向溢出。这里没有出现新的领域状态机，所以没有新增 `Users` 或 `Reports` 万能模块；它们都是 Collection + 产品行内容/动作的组合。

## Authorization 第二消费者与复用边界

Hub、Blog、Docs 等站点都使用 Foundation Authorization 领域和现有 `AuthorizationUser`、`AuthorizationApplication`、`AuthorizationGrantBadge` UI 原语。Yotta 首次迁移后，Blog 又作为第二个真实消费者验证了哪些部分应该继续下沉、哪些部分仍属于产品策略。

2026-09-14 已完成两轮收敛：

1. Yotta `AuthorizationManager` 删除不可到达的旧“用户管理”聚合、筛选、授权/撤销和批量状态，申请审核改用 `CollectionPanel + AuthorizationApplication`；独立用户管理继续由 `admin/users.vue` 负责。
2. Blog `manage/authorization.vue` 的申请审核也迁到 `CollectionPanel + AuthorizationApplication`，已授权用户列表迁到 `CollectionPanel + AuthorizationUser + AuthorizationGrantBadge`。Blog 继续拥有 identity lookup、角色名称、review/grant/revoke API 与用户授权聚合。
3. Blog 原先锁定的旧 `@yueli/ui` 候选包缺少 `AuthorizationGrantBadge` 导出和当前 Comments `columns` 合同；第二消费者改为使用 Foundation 当前源码打出的独立 tarball 候选包，没有引入跨仓源码依赖。Foundation `test:pack`、Blog typecheck、21 个产品测试与 production build 均通过。
4. 角色/能力编辑没有继续抽成 `AuthorizationConsole`。Hub 在首次修改时自动创建草稿，并把“是否允许用户申请角色”建模为 assignment source；Blog 要求显式进入草稿，还拥有“注册用户自动成为作者”自动规则和 invitation source。两者的策略生命周期已经真实证明不同，不能因为页面相似就合并状态机。
5. Hub 的真实 Workspace Playwright 已验证申请审核的共享分页、选择、批量动作和 390px 布局。Blog 对应 E2E 已改为断言公共 `data-collection-selection`、申请/用户授权原语和响应式无溢出；首次执行因本机缺少登录凭据只完成注册校验；2026-09-14 Windows 更新后已通过 Workspace Isolated 恢复正式本地组合，实际登录、集合选择/取消、角色能力页及 390/768/1280px 验收全部通过。

因此 Authorization 当前最深的稳定组合已经由两个消费者确认：Foundation Authorization 领域 + `AuthorizationApplication` / `AuthorizationUser` / `AuthorizationGrantBadge` + Collection 状态机；产品拥有 policy draft 生命周期、assignment source 规则、automatic rules、identity mapping 和 mutation。除非后续第三个消费者证明更深的策略行为也一致，否则不创建万能 Authorization Console。

## Yotta Settings 消费验证

Foundation Settings 已经在 Shop、Resource、Gallery、Blog 等产品中反复使用，拥有 `SettingsLayout`、`SettingSection`、`SettingsSaveDock`、JSON-safe dirty workflow 以及 Router/browser leave protection。Yotta 原本只消费 dirty workflow，分区和保存生命周期仍由页面自己维护。

2026-09-14 Yotta 站点设置补齐为完整消费者：

1. `SettingsLayout` 负责站点/投稿分区与响应式导航，`SettingsSaveDock` 统一未保存、保存中、成功和失败反馈；Yotta 继续提供自己的字段和保存 API。
2. `SiteSettings` 通过 Foundation `bindSettingsBeforeUnload` 与 `useSettingsLeaveGuard` 保护未保存修改，不再手写浏览器事件和 Router hook。
3. Workspace CLI Playwright 用真实本地组合验证了修改后 dock 出现、放弃后恢复 baseline、取消离开继续停留、确认离开正常导航，以及 390px 无横向溢出。

这证明 Settings 的共享 seam 已经足够深，不需要 Product Definition 携带字段 schema，也不需要各产品复制保存按钮、离开确认和脏状态逻辑。

## Yotta Comments 消费验证

Yotta 的评分与评论后台原本自己维护搜索、状态筛选、loading/error/empty、分页、页内选择、批量区、响应式行和选择限制。Foundation `CommentModerationCollection` 已经拥有这套评论审核集合 anatomy，因此 Yotta 只保留产品数据映射和审核动作。

2026-09-14 的迁移保持 Yotta 的评分与审核语义不变：

1. `admin/comments.vue` 把 Yotta comment/reply 映射成 `CommentModerationItem`，Foundation 负责集合状态、响应式列、分页、选择和批量操作区；作品来源、隐藏/恢复 API、required reason/revision 仍由 Yotta Adapter 负责。
2. 1–5 星评分作为紧凑的产品内容元数据保留在 comment projection 中，没有为了一个产品向 Foundation 增加通用 metadata/extension bag。
3. Foundation Comments 的 selection contract 增加可选 `isSelectable(id)`，并下传到 `CollectionPanel.isItemSelectable`。这让用户已删除评论仍可见，但不能进入批量选择；该能力是通用集合约束，不包含 Yotta 状态名。
4. Foundation focused test 与 `@yueli/ui` typecheck 通过；Yotta Hub typecheck/build、Go race/vet、contracts 通过。Workspace 真实 Hub Web 的 CLI Playwright 验证了 `/admin/comments`、评分展示、删除项不可选、批量隐藏/恢复、取消选择和 390px 无横向溢出。

这次消费再次确认 Comments 应拥有评论审核集合的完整公共交互，而产品通过 Adapter 提供领域状态、来源、额外内容和 mutation，不需要各站复制 Collection 状态机。

投稿/审核虽然视觉相似，目前不作为新的完整 Admin Module：Gallery 还有异步处理、安全状态和批量审核，BVideo 允许审核时修改站内标题/推荐语/分类并仅保存，Yotta 处理付费不可变版本与销售变更。它们继续复用 Collection、Feedback、Editor 等原语，等待第二个真正相同的 review 状态机。

## BVideo 第二消费者

BVideo 的管理员筛选系统故意没有 Primary Category 或自由 Tag，而是“多个筛选维度 + 每个维度的多级树”。这使它成为 Facet / FacetValue 的直接消费者，而不是把产品模型硬套成 Category。

2026-09-14 的第二消费者验证收紧了 Classification seam：

1. `ThemeTagGroup` 通过薄 Adapter 映射为 canonical Facet，`ThemeTag` 映射为 FacetValue。Foundation 新增按 Facet 隔离的确定性递归树、path/ancestor 投影，以及排除自身与后代的循环安全父级候选；跨 Facet 的非法父级按 orphan 暴露，不把另一维度混进树中。
2. BVideo `manage/tags.vue` 不再维护自己的树遍历和后代过滤，但创建/更新/删除、影响预览、事务和重排写入仍调用 BVideo 自己的 `/api/v1/admin/tag-groups` 与 `/api/v1/admin/tags`。
3. BVideo 后台静态导航、搜索和 current context 改由 Product Definition 投影。待审核视频数字属于运行时产品数据，BVideo 在 projection 结果上装饰 badge；这验证了 Definition 不需要演变成运行时低代码对象。
4. Foundation 37 个 UI 测试文件 / 126 个测试、typecheck、真实 tarball → 独立 Nuxt consumer production build 通过；BVideo typecheck、production build、Workspace Isolated 四进程健康检查通过。CLI Playwright 桌面真实生命周期通过“维度 → 一级 → 二级 → 三级”的创建、父级移动、重排、公开展示和子树删除；移动端 Classification 编辑旅程也通过。

因此 Facet/FacetValue 的层级语义已经有两个层面的证据：Foundation 本身拥有该领域，BVideo 是真实递归矩阵消费者。

## Yotta FacetValue / binding 边界验证

Yotta `filter-dimensions` 随后作为第三个真实 Classification 证据验证了“共享 Facet 语义”和“产品绑定策略”的边界：

1. 本地维度与公共维度都复用 `buildAdminFacetValueTree` 和 `createAdminFacetValueParentOptions`。Yotta 只保留 `FilterValue -> AdminClassificationFacetValue` 的薄 Adapter，因此后台树顺序、层级、path 和“不可把父级移动到自己/后代下面”不再由 Yotta 重写。
2. `minValues`、`maxValues`、`leafOnly`、`maxDepth` 属于 canonical Facet assignment policy。Foundation Go Classification 已经验证这些范围并在 classify 阶段执行它们，所以它们不是 Yotta binding 的私有字段。
3. `shared/system/local` 来源、`authorVisible`、`discoveryVisible`、`allowedValueIds`、binding revision/HTTP transport，以及“作者端隐藏时不能要求必填”继续由 Yotta 拥有。现在没有第二个产品证明这组 binding 状态机可共享，因此 Foundation 不增加通用 binding 抽象。
4. Yotta 运行时的 `filterRows` 仍留在产品里，因为它还计算 `available`、`leaf` 和发布/发现时的产品状态；后台管理树改用 Foundation projection 并不意味着运行时筛选也应被抽走。
5. CLI Playwright 用真实 Workspace 组合验证了公共维度树顺序与缩进、父级候选不泄露自身/后代、author/discovery visibility、allowed subtree、系统 Facet 只读视图、390px 无横向溢出和零 `pageerror`。Hub production build 同时通过。

这次验证之后 HTTP seam 已经落地：Foundation `classification/adminapi` 拥有 canonical Category/Facet/FacetValue/Tag、policy 与 mutation wire shape；Yotta Registry 的分类/标签管理接口已经迁到 `/v1/admin/classification/{kind}/...`，旧未发布 category/tag 管理格式没有保留兼容层。Yotta local/shared dimension 与 binding 接口继续是产品扩展；待 Foundation Go 正式 release 后，Registry 的临时同形 wire declaration 直接替换成 release import，不增加本地 `replace` 或跨仓源码依赖。
