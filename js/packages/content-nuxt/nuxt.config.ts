// @platform/content — shared content kit Nuxt layer (rich editor + prose renderer).
// Consumers `extends: ['@platform/content']` and get the components/composables
// auto-imported PLUS this build config merged in — so they never re-declare the
// prosemirror dedup / katex css. See checklists/frontend-dev-gotchas §7.
export default defineNuxtConfig({
  // katex stylesheet is global so it covers both the reading side and the
  // editor's math node previews (E4).
  css: ['katex/dist/katex.min.css'],
  // tiptap editor dedup: the directly-imported @tiptap/* extensions and @nuxt/ui's
  // bundled tiptap must share ONE prosemirror instance, else Vite pre-bundles two
  // copies of prosemirror-state and the editor throws "Adding different instances
  // of a keyed plugin (plugin$)". Forcing both into one optimized chunk fixes it.
  vite: {
    optimizeDeps: {
      include: [
        '@nuxt/ui > prosemirror-state',
        '@nuxt/ui > prosemirror-transform',
        '@nuxt/ui > prosemirror-model',
        '@nuxt/ui > prosemirror-view',
        '@nuxt/ui > prosemirror-gapcursor',
        '@platform/content > @tiptap/core',
        '@platform/content > @tiptap/vue-3',
        '@platform/content > @tiptap/extension-emoji',
        '@platform/content > @tiptap/extension-text-align',
        '@platform/content > @tiptap/extension-mathematics',
        '@platform/content > @tiptap/extension-blockquote',
        '@platform/content > highlight.js',
        '@platform/content > katex',
        '@platform/content > marked',
        '@platform/content > marked-alert'
      ]
    }
  }
})
