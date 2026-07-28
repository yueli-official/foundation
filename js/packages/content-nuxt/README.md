# @yueli/content-nuxt

Foundation 所有的内容工具 Nuxt layer，向 Blog、Docs、Resource、Shop 等内容站共享富 Markdown 编辑器和
阅读端渲染器。它不调用站点接口；消费应用通过 `image-uploader` adapter 提供上传，通过自己的领域流程保存正文。

## 模块边界

- 外部接口只有 `<ContentEditor>`、`<ContentProse>` 与 `article.css`。
- Markdown 渲染器、Tiptap 编辑器扩展、Mermaid 图表渲染器和本地草稿属于模块内部实现，
  不拆成要求消费者重新组合的浅包。
- 产品拥有文章、商品和资源条目的数据结构、权限、发布生命周期与远程保存；本模块只拥有通用内容行为。

## 自动导入内容

| 组件              | 用途                                                                                                    | 接口                                                                                                                          |
| ----------------- | ------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `<ContentEditor>` | 完整富文本 Markdown 编辑器，支持代码高亮、数学公式、Mermaid、提示块、表情、拖拽手柄和草稿自动保存。     | `v-model` 为 Markdown 字符串；属性包括 `image-uploader`、`draft-entity-id`、`has-initial-content`；ref 方法为 `markSaved()`。 |
| `<ContentProse>`  | 使用 marked、KaTeX、highlight.js、Mermaid 与 marked-alert 渲染相同 Markdown，使用隔离的 `Marked` 实例。 | `content: string`。                                                                                                           |

消费应用扩展该 layer 后可直接在模板使用两个组件，无需显式导入。

## 接入方式

1. 在应用 `package.json` 增加工作区依赖：

   ```json
   { "dependencies": { "@yueli/content-nuxt": "<正式 Release URL>" } }
   ```

2. 在 `nuxt.config.ts` 扩展 layer：

   ```ts
   export default defineNuxtConfig({
     extends: ["@yueli/identity-nuxt", "@yueli/content-nuxt"],
   });
   ```

3. 在应用全局 CSS 中、Tailwind typography 插件之后导入文章样式。顺序不能反，因为文章样式需要覆盖 typography：

   ```css
   @plugin "@tailwindcss/typography";
   @import "@yueli/content-nuxt/article.css"; /* 代码高亮、数学公式与提示块 */
   ```

## Layer 已统一处理的事项

消费者不得重复声明以下配置；Nuxt `extends` 会合并本 layer 的 `nuxt.config`：

- **ProseMirror 单实例去重**：`vite.optimizeDeps.include` 强制直接导入的 `@tiptap/*` 扩展与 `@nuxt/ui` 内置 Tiptap 共享同一 ProseMirror，避免 `Adding different instances of a keyed plugin`。
- **KaTeX 样式**：全局引入 `katex/dist/katex.min.css`，同时覆盖阅读器和编辑器公式预览。
- **隔离 marked**：`<ContentProse>` 使用私有 `new Marked()`，不会污染 `@nuxt/ui` 编辑器解析器。

消费者唯一需要手工完成的是上面的 `article.css` 导入，因为全局注入无法保证它与 typography 插件的相对顺序。

## 离线与依赖

- 本层会启用 `@nuxt/ui`，消费应用必须提供满足版本范围的对等依赖，因为 `useEditorToolbar` 在运行时导入
  `@nuxt/ui/utils/editor`。
- 可离线使用：Tabler 图标、KaTeX 与 highlight.js 样式都随 `article.css` 提供，不依赖 CDN。
- Tiptap 固定为与 `@nuxt/ui` 4.9.0 内置版本一致的 `3.28.0`，同时携带 `@tiptap/pm` 与表情菜单所需的
  `@tiptap/suggestion`，不把编辑器内部 peer 留给消费应用补齐。

验证：

```text
pnpm --filter @yueli/content-nuxt test
pnpm --filter @yueli/content-nuxt typecheck
pnpm --filter @yueli/content-nuxt pack --dry-run
```
