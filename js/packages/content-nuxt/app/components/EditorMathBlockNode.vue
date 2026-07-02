<script setup lang="ts">
import { NodeViewWrapper } from '@tiptap/vue-3'
import katex from 'katex'

// Editor-side NodeView for MathBlock (E4): click-to-edit LaTeX with a live KaTeX
// preview. Ported from donor.
const props = defineProps<{
  node: { attrs: { latex: string } }
  updateAttributes: (attrs: Record<string, unknown>) => void
  selected: boolean
}>()

const editing = ref(false)
const inputRef = ref<HTMLTextAreaElement>()

const rendered = computed(() => {
  const src = props.node.attrs.latex || ''
  if (!src.trim()) return '<span class="text-muted text-sm">点击输入公式</span>'
  try {
    return katex.renderToString(src, { displayMode: true, throwOnError: false })
  } catch {
    return `<span class="text-error text-sm">${src}</span>`
  }
})

function startEdit() {
  editing.value = true
  nextTick(() => {
    inputRef.value?.focus()
    autoResize()
  })
}

function onInput(e: Event) {
  const val = (e.target as HTMLTextAreaElement).value
  props.updateAttributes({ latex: val })
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
  // Escape exits editing
  if (e.key === 'Escape') {
    editing.value = false
    e.preventDefault()
  }
}
</script>

<template>
  <NodeViewWrapper class="math-block-editor" :class="{ 'is-selected': selected }">
    <!-- Edit area -->
    <div v-if="editing" class="math-edit-area">
      <div class="math-edit-header" contenteditable="false">
        <span class="math-label">LaTeX</span>
      </div>
      <textarea
        ref="inputRef"
        :value="node.attrs.latex"
        class="math-input"
        spellcheck="false"
        placeholder="E = mc^2"
        @input="onInput"
        @blur="onBlur"
        @keydown="onKeydown" />
    </div>
    <!-- Preview -->
    <div
      class="math-preview"
      :class="{ 'math-preview--clickable': !editing }"
      contenteditable="false"
      @click="!editing && startEdit()">
      <div v-html="rendered" />
    </div>
  </NodeViewWrapper>
</template>

<style scoped>
.math-block-editor {
  margin: 1em 0;
  border: 1px solid var(--ui-border);
  border-radius: 0.75rem;
  overflow: hidden;
  transition: border-color 0.15s;
}
.math-block-editor.is-selected,
.math-block-editor:focus-within {
  border-color: var(--ui-primary);
}
.math-edit-area {
  border-bottom: 1px solid var(--ui-border);
}
.math-edit-header {
  display: flex;
  align-items: center;
  padding: 0.25rem 0.75rem;
  background: var(--ui-bg-elevated);
  border-bottom: 1px solid var(--ui-border);
}
.math-label {
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--ui-text-muted);
}
.math-input {
  width: 100%;
  min-height: 2.5em;
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
.math-preview {
  padding: 1em;
  text-align: center;
  overflow-x: auto;
}
.math-preview--clickable {
  cursor: pointer;
  min-height: 3em;
  display: flex;
  align-items: center;
  justify-content: center;
}
.math-preview--clickable:hover {
  background: var(--ui-bg-elevated);
}
</style>
