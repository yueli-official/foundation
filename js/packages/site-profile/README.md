# `@yueli/site-profile`

`@yueli/site-profile` 是 Foundation Site Profile 合同的无头管理适配层。

它负责：

- 校验服务端返回的 Form Schema 与 Snapshot；
- 按 schema 查找字段并安全访问 dotted path；
- 管理草稿、dirty/reset 状态；
- 使用 `If-Match` 提交完整替换请求；
- 通过可选 Vue export 提供与同一状态模型绑定的编辑器。

产品继续拥有页面布局、文案、授权和保存反馈；本包不拥有站点品牌策略或管理页面。

## 使用

```ts
import { SiteProfileEditor } from "@yueli/site-profile";
import type { SiteProfile } from "@yueli/site-profile/types";
```

只有需要 Vue 编辑器的消费者才需要安装 Vue peer dependency。修改 schema/snapshot 校验、路径访问或
`If-Match` 行为时，运行：

```text
pnpm --filter @yueli/site-profile typecheck
pnpm --filter @yueli/site-profile test
```
