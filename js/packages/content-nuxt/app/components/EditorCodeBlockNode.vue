<script setup lang="ts">
import { NodeViewWrapper } from '@tiptap/vue-3'
import hljs from 'highlight.js'
import { DEFAULT_CODE_LANGUAGES } from '../utils/codeLanguages'

// Editor-side NodeView for CodeBlockWithLang (E3): language picker + click-to-edit
// textarea + highlight.js preview. Token colors come from the global highlight
// theme in assets/css/article.css (offline, no CDN). Ported from donor.
const props = defineProps<{
  node: { attrs: { language: string | null, code: string } }
  updateAttributes: (attrs: Record<string, unknown>) => void
  selected: boolean
}>()

const languages = DEFAULT_CODE_LANGUAGES

const editing = ref(false)
const inputRef = ref<HTMLTextAreaElement>()

// Auto-enter edit mode when code is empty
watch(
  () => props.node.attrs.code,
  (code) => {
    if (!code && !editing.value) {
      startEdit()
    }
  },
  { immediate: true },
)

const highlighted = computed(() => {
  const code = props.node.attrs.code || ''
  if (!code.trim()) return ''
  const lang = props.node.attrs.language
  try {
    if (lang && hljs.getLanguage(lang)) {
      return hljs.highlight(code, { language: lang }).value
    }
    return hljs.highlightAuto(code).value
  } catch {
    return escapeHtml(code)
  }
})

function escapeHtml(text: string) {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

function onLangChange(event: Event) {
  const val = (event.target as HTMLSelectElement).value
  props.updateAttributes({ language: val || null })
}

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
  <NodeViewWrapper class="code-block-with-lang" :class="{ 'is-selected': selected }">
    <div class="code-block-header" contenteditable="false">
      <select
        :value="node.attrs.language || ''"
        class="lang-select"
        @change="onLangChange">
        <option
          v-for="lang in languages"
          :key="lang.value"
          :value="lang.value">
          {{ lang.label }}
        </option>
      </select>
    </div>
    <!-- Edit mode -->
    <div v-if="editing" class="code-edit-area">
      <textarea
        ref="inputRef"
        :value="node.attrs.code"
        class="code-input"
        spellcheck="false"
        placeholder="点击输入代码…"
        @input="onInput"
        @blur="onBlur"
        @keydown="onKeydown" />
    </div>
    <!-- Preview mode -->
    <div
      v-else
      class="code-preview"
      :class="{ 'code-preview--clickable': !editing }"
      contenteditable="false"
      @click="startEdit()">
      <div v-if="!node.attrs.code?.trim()" class="text-muted text-sm code-placeholder">
        点击输入代码…
      </div>
      <pre v-else><code class="hljs" v-html="highlighted" /></pre>
    </div>
  </NodeViewWrapper>
</template>

<style scoped>
.code-block-with-lang {
  position: relative;
  margin: 1em 0;
  border: 1px solid var(--ui-border);
  border-radius: 0.75rem;
  overflow: hidden;
  transition: border-color 0.15s;
}
.code-block-with-lang.is-selected,
.code-block-with-lang:focus-within {
  border-color: var(--ui-primary);
}
.code-block-header {
  display: flex;
  justify-content: flex-end;
  padding: 0.25rem 0.5rem;
  background: var(--ui-bg-elevated);
  border-bottom: 1px solid var(--ui-border);
}
.lang-select {
  font-size: 0.75rem;
  padding: 0.125rem 0.375rem;
  border-radius: 0.25rem;
  border: 1px solid var(--ui-border);
  background: var(--ui-bg);
  color: var(--ui-text-muted);
  cursor: pointer;
  outline: none;
}
.lang-select:focus {
  border-color: var(--ui-primary);
}
.code-edit-area {
  background: var(--ui-bg);
}
.code-input {
  width: 100%;
  min-height: 3em;
  padding: 0.75em 1em;
  font-family: "JetBrains Mono", "Courier New", monospace;
  font-size: 0.875em;
  line-height: 1.5;
  background: transparent;
  color: var(--ui-text);
  border: none;
  outline: none;
  resize: none;
  overflow: hidden;
}
.code-preview pre {
  margin: 0;
  padding: 1em;
  overflow-x: auto;
  background: transparent;
  border: none;
  border-radius: 0;
}
.code-preview pre code {
  font-size: 0.875em;
  font-family: "JetBrains Mono", "Courier New", monospace;
}
.code-preview--clickable {
  cursor: pointer;
  min-height: 3em;
}
.code-preview--clickable:hover {
  background: var(--ui-bg-elevated);
}
.code-placeholder {
  padding: 1em;
  text-align: center;
}
</style>
