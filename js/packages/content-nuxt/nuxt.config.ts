// @yueli/content-nuxt — shared content kit Nuxt layer.
// Consumers extend the layer and get the components/composables
// auto-imported PLUS this build config merged in — so they never re-declare the
// prosemirror dedup / katex css. See checklists/frontend-dev-gotchas §7.
export default defineNuxtConfig({
  modules: ['@nuxt/ui'],
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
        '@yueli/content-nuxt > @tiptap/core',
        '@yueli/content-nuxt > @tiptap/vue-3',
        '@yueli/content-nuxt > @tiptap/extension-emoji',
        '@yueli/content-nuxt > @tiptap/extension-text-align',
        '@yueli/content-nuxt > @tiptap/extension-mathematics',
        '@yueli/content-nuxt > @tiptap/extension-blockquote',
        '@yueli/content-nuxt > highlight.js',
        '@yueli/content-nuxt > katex',
        '@yueli/content-nuxt > marked',
        '@yueli/content-nuxt > marked-alert'
      ]
    }
  }
})
