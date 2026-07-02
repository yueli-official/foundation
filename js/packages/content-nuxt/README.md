# @platform/content

A site-agnostic **content kit** Nuxt layer: the rich markdown editor and the
reading-side renderer, shared across content sites (blog / resource / mall). It
carries data contracts only (v-model body, cover props) — no site API calls; each
consuming app feeds it data via its own `useApi`.

## What it provides (auto-imported)

| Component | Purpose | API |
|---|---|---|
| `<ContentEditor>` | Full rich-text markdown editor (UEditor + custom nodes: code-highlight, math, mermaid, callouts; emoji / drag-handle / draft auto-save). | `v-model` → body markdown `string`; props `image-uploader: (f: File) => Promise<string>`, `draft-entity-id: string \| number`, `has-initial-content: boolean`; ref method `markSaved()`. |
| `<ContentProse>` | Reading-side render of the same markdown (marked + katex + highlight.js + mermaid + marked-alert, isolated `Marked` instance). | prop `content: string`. |

Both are Nuxt auto-imported — `extends` the layer and use them in templates with no
explicit import.

## How to consume

1. Add the workspace dep in the app's `package.json`:

   ```json
   { "dependencies": { "@platform/content": "workspace:*" } }
   ```

2. Extend the layer in `nuxt.config.ts`:

   ```ts
   export default defineNuxtConfig({
     extends: ['@platform/auth', '@platform/content'],
   })
   ```

3. Import the article stylesheet in the app's global CSS, **after** the Tailwind
   typography plugin (ordering matters — the article styles override typography):

   ```css
   @plugin "@tailwindcss/typography";
   @import "@platform/content/article.css";  /* code highlight / math / callouts */
   ```

That's it — `<ContentEditor>` / `<ContentProse>` are then available in templates.

## What the layer handles for you

Consumers do **not** re-declare any of these — the layer's `nuxt.config` is merged
into the consumer by Nuxt `extends` (verified: the layer's `optimizeDeps.include`
lands in the consumer's Vite dep-optimize metadata):

- **prosemirror single-instance dedup** (`vite.optimizeDeps.include`): the directly
  imported `@tiptap/*` extensions and `@nuxt/ui`'s bundled tiptap must share one
  prosemirror, else the editor throws `Adding different instances of a keyed plugin
  (plugin$)`. The layer forces them into one optimized chunk.
- **katex stylesheet** (`css: ['katex/dist/katex.min.css']`): global, covers both the
  reader and the editor's math previews.
- **isolated `marked`**: `<ContentProse>` uses a private `new Marked()` instance so it
  never pollutes `@nuxt/ui`'s editor parser.

The only thing a consumer must do by hand is the `article.css` `@import` (step 3) —
its ordering relative to your typography plugin can't be guaranteed if injected
globally, so it stays a consumer-side import.

## Offline / requirements

- Requires `@nuxt/ui` in the consumer app (every content site already registers it
  via `modules: ['@nuxt/ui']`). The layer also declares it as a dependency because
  `useEditorToolbar` imports `@nuxt/ui/utils/editor` at runtime.
- Offline-safe: tabler icons, and katex/highlight.js CSS ship inside `article.css` —
  no CDN.
- tiptap is pinned `^3.27.0` (matches `@nuxt/ui` 4.x's bundled tiptap); `@tiptap/pm`
  is carried alongside.
