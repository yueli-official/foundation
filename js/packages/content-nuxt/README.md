# @platform/content

与站点无关的内容工具 Nuxt layer，向 Blog、Resource、Shop 等内容站共享富 Markdown 编辑器和阅读端渲染器。它只携带数据契约（正文 `v-model`、封面属性），不调用任何站点接口；消费应用通过自己的 `useApi` 提供数据。

## 自动导入内容

| 组件 | 用途 | Interface |
| --- | --- | --- |
| `<ContentEditor>` | 完整富文本 Markdown 编辑器，支持代码高亮、数学公式、Mermaid、提示块、表情、拖拽手柄和草稿自动保存。 | `v-model` 为 Markdown 字符串；属性包括 `image-uploader`、`draft-entity-id`、`has-initial-content`；ref 方法为 `markSaved()`。 |
| `<ContentProse>` | 使用 marked、KaTeX、highlight.js、Mermaid 与 marked-alert 渲染相同 Markdown，使用隔离的 `Marked` 实例。 | `content: string`。 |

消费应用扩展该 layer 后可直接在模板使用两个组件，无需显式导入。

## 接入方式

1. 在应用 `package.json` 增加工作区依赖：

   ```json
   { "dependencies": { "@platform/content": "workspace:*" } }
   ```

2. 在 `nuxt.config.ts` 扩展 layer：

   ```ts
   export default defineNuxtConfig({
     extends: ['@yueli/identity-nuxt', '@platform/content'],
   })
   ```

3. 在应用全局 CSS 中、Tailwind typography 插件之后导入文章样式。顺序不能反，因为文章样式需要覆盖 typography：

   ```css
   @plugin "@tailwindcss/typography";
   @import "@platform/content/article.css";  /* 代码高亮、数学公式与提示块 */
   ```

## Layer 已统一处理的事项

消费者不得重复声明以下配置；Nuxt `extends` 会合并本 layer 的 `nuxt.config`：

- **ProseMirror 单实例去重**：`vite.optimizeDeps.include` 强制直接导入的 `@tiptap/*` 扩展与 `@nuxt/ui` 内置 Tiptap 共享同一 ProseMirror，避免 `Adding different instances of a keyed plugin`。
- **KaTeX 样式**：全局引入 `katex/dist/katex.min.css`，同时覆盖阅读器和编辑器公式预览。
- **隔离 marked**：`<ContentProse>` 使用私有 `new Marked()`，不会污染 `@nuxt/ui` 编辑器解析器。

消费者唯一需要手工完成的是上面的 `article.css` 导入，因为全局注入无法保证它与 typography 插件的相对顺序。

## 离线与依赖

- 消费应用必须启用 `@nuxt/ui`；本 layer 也声明它为依赖，因为 `useEditorToolbar` 在运行时导入 `@nuxt/ui/utils/editor`。
- 可离线使用：Tabler 图标、KaTeX 与 highlight.js 样式都随 `article.css` 提供，不依赖 CDN。
- Tiptap 固定为与 `@nuxt/ui` 4.x 内置版本一致的 `^3.27.0`，同时携带 `@tiptap/pm`。

验证：`pnpm --filter @platform/content typecheck`，并运行消费产品的编辑器交互测试。
