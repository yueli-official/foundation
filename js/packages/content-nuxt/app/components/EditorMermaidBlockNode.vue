<script setup lang="ts">
import { NodeViewWrapper } from '@tiptap/vue-3'

// Editor-side NodeView for MermaidBlock (E5): click-to-edit Mermaid source with a
// debounced live SVG preview (mermaid lazy-loaded). Ported from donor.
const props = defineProps<{
  node: { attrs: { code: string } }
  updateAttributes: (attrs: Record<string, unknown>) => void
  selected: boolean
}>()

const editing = ref(false)
const inputRef = ref<HTMLTextAreaElement>()
const svgHtml = ref('')
const renderError = ref('')

const colorMode = useColorMode()

// Lazy-load mermaid (module cached; theme re-applied per render so it follows
// the current color mode instead of being frozen at first render).
let mermaidInstance: typeof import('mermaid')['default'] | null = null

async function getMermaid() {
  if (!mermaidInstance) {
    const mod = await import('mermaid')
    mermaidInstance = mod.default
  }
  mermaidInstance.initialize({ startOnLoad: false, theme: colorMode.value === 'dark' ? 'dark' : 'default' })
  return mermaidInstance
}

// Debounced render
let renderTimer: ReturnType<typeof setTimeout> | null = null

async function renderMermaid(code: string) {
  if (!code.trim()) {
    svgHtml.value = ''
    renderError.value = ''
    return
  }
  try {
    const mermaid = await getMermaid()
    const id = `mermaid-editor-${Math.random().toString(36).slice(2, 8)}`
    const { svg } = await mermaid.render(id, code)
    svgHtml.value = svg
    renderError.value = ''
  } catch (e: any) {
    renderError.value = e?.message || '渲染失败'
  }
}

watch(
  () => props.node.attrs.code,
  (code) => {
    if (renderTimer) clearTimeout(renderTimer)
    renderTimer = setTimeout(() => renderMermaid(code), 500)
  },
  { immediate: true },
)

// Re-render with the matching theme when the user toggles light/dark.
watch(() => colorMode.value, () => renderMermaid(props.node.attrs.code))

function startEdit() {
  editing.value = true
  nextTick(() => {
    inputRef.value?.focus()
    autoResize()
  })
}

function onInput(e: Event) {
  const val = (e.target as HTMLTextAreaElement).value
  props.updateAttributes({ code: val })
  autoResize()
}

function autoResize() {
  const el = inputRef.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = el.scrollHeight + 'px'
}

function onBlur() {
  editing.value = false
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    editing.value = false
    e.preventDefault()
  }
}
</script>

<template>
  <NodeViewWrapper class="mermaid-block-editor" :class="{ 'is-selected': selected }">
    <!-- Edit area -->
    <div v-if="editing" class="mermaid-edit-area">
      <div class="mermaid-edit-header" contenteditable="false">
        <span class="mermaid-label">Mermaid</span>
      </div>
      <textarea
        ref="inputRef"
        :value="node.attrs.code"
        class="mermaid-input"
        spellcheck="false"
        placeholder="graph TD&#10;    A--&gt;B"
        @input="onInput"
        @blur="onBlur"
        @keydown="onKeydown" />
    </div>
    <!-- Preview -->
    <div
      class="mermaid-preview"
      :class="{ 'mermaid-preview--clickable': !editing }"
      contenteditable="false"
      @click="!editing && startEdit()">
      <!-- Empty placeholder -->
      <div v-if="!node.attrs.code?.trim() && !editing" class="text-muted text-sm">
        点击输入 Mermaid 图表代码
      </div>
      <!-- Error -->
      <div v-else-if="renderError" class="mermaid-error">
        <div class="text-error text-sm mb-2">{{ renderError }}</div>
        <pre class="text-xs text-muted">{{ node.attrs.code }}</pre>
      </div>
      <!-- SVG -->
      <div v-else-if="svgHtml" v-html="svgHtml" />
      <!-- Loading -->
      <div v-else class="text-muted text-sm">…</div>
    </div>
  </NodeViewWrapper>
</template>

<style scoped>
.mermaid-block-editor {
  margin: 1em 0;
  border: 1px solid var(--ui-border);
  border-radius: 0.75rem;
  overflow: hidden;
  transition: border-color 0.15s;
}
.mermaid-block-editor.is-selected,
.mermaid-block-editor:focus-within {
  border-color: var(--ui-primary);
}
.mermaid-edit-area {
  border-bottom: 1px solid var(--ui-border);
}
.mermaid-edit-header {
  display: flex;
  align-items: center;
  padding: 0.25rem 0.75rem;
  background: var(--ui-bg-elevated);
  border-bottom: 1px solid var(--ui-border);
}
.mermaid-label {
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--ui-text-muted);
}
.mermaid-input {
  width: 100%;
  min-height: 4em;
  padding: 0.75em;
  font-family: "JetBrains Mono", "Courier New", monospace;
  font-size: 0.875em;
  line-height: 1.5;
  background: var(--ui-bg);
  color: var(--ui-text);
  border: none;
  outline: none;
  resize: none;
  overflow: hidden;
}
.mermaid-preview {
  padding: 1em;
  text-align: center;
  overflow-x: auto;
}
.mermaid-preview--clickable {
  cursor: pointer;
  min-height: 3em;
  display: flex;
  align-items: center;
  justify-content: center;
}
.mermaid-preview--clickable:hover {
  background: var(--ui-bg-elevated);
}
.mermaid-error {
  text-align: left;
  padding: 0.5em;
}
.mermaid-error pre {
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
