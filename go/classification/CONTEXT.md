# Classification

Classification 为图片、文章、资源、商品及其他内容消费者提供统一的 Category、Facet、Facet Value 和 Tag 规则核，同时让每个消费者持有自己的 Catalog、数据与事务。

## Language

**Catalog**：消费者隔离的分类结构与 Policy 快照；以数据库 revision 编译为不可变内存对象。

**Category**：支持多归属和单父层级的主要分类；父分类筛选包含全部后代。

**Primary Category**：消费者从对象现有 Category Assignment 中显式选择的主要入口；不由数组顺序推断。

**Facet / Facet Value**：可组合的受控筛选轴及其可分层值；对象归类基数与查询多选相互独立。

**Tag**：扁平的长尾规范词；canonical、Alias 与 replacement 共用 Lookup Registry，未知输入由消费者 Policy 决定 create、propose 或 reject。

**Filter Plan**：一个可选 Category Group 与多个 Facet Group；组内 OR、组间 AND，层级引用在执行前展开为稳定 ID。

**Contextual Candidate Count**：计算某组候选时排除该组、保留其他全部条件，并按 distinct object 计数。

**Governance Plan**：对 lifecycle、reparent、merge 或 delete 的纯计划；携带 Catalog revision 与 impact freshness token，由消费者事务执行器复核后落库。

## Public seam

- `Compile(Snapshot)`：校验结构、replacement、Policy 和层级不变量，返回 concrete immutable `*Catalog`。
- `Catalog.Classify`：规范对象的 Category、Primary、Facet Value 与 Tag 写入。
- `Catalog.Discover`：规范公开引用，生成 Filter Plan 与候选投影。
- `Catalog.Govern`：生成可预览的治理计划。
- 三个意图都使用 typed `Preparation → Facts → Complete`；基础设施错误不进入领域 diagnostics。

## Ownership

- Foundation Classification 是规则权威，不持有数据库、HTTP、事务或 Redis。
- 消费者负责 Snapshot/Tag/count/impact adapter、Assignment 表、Primary companion 表、outbox、NOTIFY 与 30 秒 revision 检查。
- Gallery 是完整治理消费者参考实现；Blog 和 Resource 证明无 Facet/Primary 的不同 Policy 与 content PostgreSQL 持久化形态。三个消费者分别拥有表、事务执行器和 Policy。
- TypeScript 只消费 transport DTO 和 diagnostics，不复制层级、normalization、replacement 或 Policy kernel。

## Invariants

- Category 与 Facet Value 是单父森林；Facet Value 的 parent/replacement 必须保持同 Facet。
- inactive/replaced identity 不接受新 Assignment；父级或 owner inactive 通过 effective-active 派生隐藏子树。
- Category、Facet 与 Facet Value 首次进入 active 时由消费者记录 `first_activated_at`；之后停用和恢复不得覆盖它。
- Primary companion 使用指向 Category Assignment 的 deferred 复合外键。
- Tag Lookup Key 使用 NFKC、空白折叠与大小写折叠；不自动翻译、转写或猜测同义词。
- 删除不依赖隐式 cascade 语义；有引用时必须预览并显式确认 delete-all-related。
- 当前 Governance Plan 不表达跨 Facet 的 Value 映射，因此不支持直接合并整个 Facet；应先治理 Value，再删除空 Facet。

完整领域决策和持久化模板保存在 Platform 的
`flightdeck/work/2026-07-15-unified-classification-model/` 历史 Work 中。
