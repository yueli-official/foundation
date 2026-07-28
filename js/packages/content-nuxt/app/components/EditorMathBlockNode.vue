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
  <NodeViewWrapper
    class="my-[1em] overflow-hidden rounded-xl border transition-colors duration-150 focus-within:border-primary"
    :class="selected ? 'border-primary' : 'border-default'">
    <!-- Edit area -->
    <div v-if="editing" class="border-b border-default">
      <div class="flex items-center border-b border-default bg-elevated px-3 py-1" contenteditable="false">
        <span class="text-[0.7rem] font-semibold uppercase tracking-[0.05em] text-muted">LaTeX</span>
      </div>
      <textarea
        ref="inputRef"
        :value="node.attrs.latex"
        class="min-h-[2.5em] w-full resize-none overflow-hidden border-0 bg-default p-[0.75em] font-mono text-[0.875em] leading-6 text-default outline-none"
        spellcheck="false"
        placeholder="E = mc^2"
        @input="onInput"
        @blur="onBlur"
        @keydown="onKeydown" />
    </div>
    <!-- Preview -->
    <div
      class="overflow-x-auto p-[1em] text-center"
      :class="{ 'flex min-h-[3em] cursor-pointer items-center justify-center hover:bg-elevated': !editing }"
      contenteditable="false"
      @click="!editing && startEdit()">
      <div v-html="rendered" />
    </div>
  </NodeViewWrapper>
</template>
