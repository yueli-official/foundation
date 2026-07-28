<script setup lang="ts">
// Reading-side article body: renders post markdown (highlight.js / KaTeX /
// callouts baked in by useMarkdown) and adds copy buttons to code blocks.
// Mermaid hydration lives in a .client component so Nitro never bundles Mermaid.
const props = defineProps<{ content: string }>()

const { render } = useMarkdown()
const html = computed(() => render(props.content ?? ''))

const el = ref<HTMLElement>()

// ── Copy buttons on code blocks ───────────────────────────────────────────
function addCopyButtons() {
  if (!el.value) return
  el.value.querySelectorAll('pre').forEach((pre) => {
    if (pre.querySelector('.copy-btn')) return
    if (pre.querySelector('code.language-mermaid')) return
    const btn = document.createElement('button')
    btn.className = 'copy-btn'
    btn.type = 'button'
    btn.textContent = '复制'
    btn.addEventListener('click', async () => {
      const code = pre.querySelector('code')?.textContent ?? ''
      await navigator.clipboard.writeText(code)
      btn.textContent = '已复制'
      setTimeout(() => { btn.textContent = '复制' }, 1500)
    })
    pre.style.position = 'relative'
    pre.appendChild(btn)
  })
}

function postRender() {
  addCopyButtons()
}

watch(html, () => nextTick(postRender))
onMounted(() => nextTick(postRender))
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -->
  <div
    ref="el"
    class="prose content-prose max-w-none dark:prose-invert prose-headings:font-display prose-headings:tracking-tight prose-headings:scroll-mt-24 prose-a:text-primary prose-img:rounded-lg prose-pre:rounded-lg"
    v-html="html" />
  <ContentMermaidHydrator :target="el" />
</template>
