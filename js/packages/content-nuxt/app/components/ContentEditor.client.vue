<script setup lang="ts">
import type { Editor } from '@tiptap/vue-3'
import { Emoji, gitHubEmojis } from '@tiptap/extension-emoji'
import { TextAlign } from '@tiptap/extension-text-align'
import { InlineMath } from '@tiptap/extension-mathematics'
import Blockquote from '@tiptap/extension-blockquote'
import { Callout, type CalloutType } from '../extensions/Callout'
import { CodeBlockWithLang } from '../extensions/CodeBlockWithLang'
import { MathBlock } from '../extensions/MathBlock'
import { MermaidBlock } from '../extensions/MermaidBlock'

// Full markdown rich-text editor for the blog (editor E1-E7): UEditor +
// markdown round-trip with custom nodes for code-highlight (E3), math (E4),
// mermaid (E5), callouts (E6), plus draft auto-save / emoji / drag-handle /
// word-count (E7). Mirrors the donor admin editor, minus its plugin system,
// @mention menu and link popover. Reading-side rendering lives in BlogProse.vue.
const props = withDefaults(defineProps<{
  modelValue: string
  placeholder?: string
  // (file) => uploaded public URL — wires the toolbar/“+” image action to the
  // blog asset upload. When omitted the image action is a no-op.
  imageUploader?: (file: File) => Promise<string>
  // Draft auto-save: per-entity localStorage key + whether the server already
  // had content (so a freshly-loaded post isn't shadowed by a stale local draft).
  draftEntityId?: string | number
  draftMode?: 'create' | 'edit'
  hasInitialContent?: boolean
  // localStorage namespace for the draft auto-save — set per consuming site so
  // drafts don't collide across content kinds (default keeps the blog's key).
  draftKeyPrefix?: string
}>(), {
  placeholder: '开始写作…支持 Markdown 与富文本',
  imageUploader: undefined,
  draftEntityId: undefined,
  draftMode: 'edit',
  hasInitialContent: false,
  draftKeyPrefix: 'blog:post',
})

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const toast = useToast()

// ── Extensions (mirror donor; starter-kit codeBlock/blockquote are replaced) ──
const editorExtensions = [
  Emoji,
  TextAlign.configure({ types: ['heading', 'paragraph'] }),
  // Callout intercepts the blockquote markdown token, so the real Blockquote
  // must not also parse it (would double-parse). Rendering stays default.
  Blockquote.extend({ parseMarkdown: null as any }),
  InlineMath,
  MathBlock,
  MermaidBlock,
  Callout,
  CodeBlockWithLang,
]

const appendTo = import.meta.client ? () => document.body : undefined
const emojiItems = gitHubEmojis.filter(e => !e.name.startsWith('regional_indicator_'))

// ── Toolbar config ────────────────────────────────────────────────────────
const {
  toolbarItems,
  bubbleItems,
  suggestionItems,
  imageBubbleItems,
  selectedNode,
  dragHandleItems,
} = useEditorToolbar()

// ── Image upload (toolbar “image” + suggestion menu) ──────────────────────
const editorRef = ref<any>(null)
const fileInput = ref<HTMLInputElement>()
const uploading = ref(false)

function pickImage() {
  if (!props.imageUploader) return
  fileInput.value?.click()
}

async function onFile(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  // Resolve the editor from the UEditor ref (it exposes `editor`) rather than a
  // stashed reference, so upload works regardless of how the picker was opened.
  const editor = editorRef.value?.editor as Editor | undefined
  if (!file || !editor || !props.imageUploader) return
  uploading.value = true
  try {
    const url = await props.imageUploader(file)
    editor.chain().focus().setImage({ src: url, alt: file.name }).run()
    toast.add({ title: '图片已插入', color: 'success', icon: 'i-tabler-check' })
  } catch (err: any) {
    toast.add({ title: '图片上传失败', description: err?.message || '请重试', color: 'error' })
  } finally {
    uploading.value = false
  }
}

// ── Custom command handlers for the editor toolbar / bubble / suggestion ──
const calloutHandler = (type: CalloutType) => ({
  canExecute: (editor: Editor) => editor.isEditable,
  execute: (editor: Editor) => editor.chain().focus().setCallout({ type }).run(),
  isActive: (editor: Editor) => editor.isActive('callout', { type }),
})

const handlers = {
  image: {
    canExecute: (editor: Editor) => editor.isEditable && !!props.imageUploader,
    execute: (editor: Editor) => { pickImage(); return editor.chain() },
    isActive: (_editor: Editor) => false,
  },
  'callout-note': calloutHandler('note'),
  'callout-tip': calloutHandler('tip'),
  'callout-important': calloutHandler('important'),
  'callout-warning': calloutHandler('warning'),
  'callout-caution': calloutHandler('caution'),
  'math-inline': {
    canExecute: (editor: Editor) => editor.isEditable,
    execute: (editor: Editor) => editor.chain().focus().insertContent('$E=mc^2$').run(),
    isActive: (_editor: Editor) => false,
  },
  'math-block': {
    canExecute: (editor: Editor) => editor.isEditable,
    execute: (editor: Editor) => editor.chain().focus().setMathBlock({ latex: 'E = mc^2' }).run(),
    isActive: (editor: Editor) => editor.isActive('blockMath'),
  },
  'mermaid-block': {
    canExecute: (editor: Editor) => editor.isEditable,
    execute: (editor: Editor) => editor.chain().focus().setMermaidBlock({ code: 'graph TD\n    A-->B' }).run(),
    isActive: (editor: Editor) => editor.isActive('mermaidBlock'),
  },
  'download-image': {
    canExecute: (editor: Editor) => editor.isActive('image'),
    execute: (editor: Editor) => {
      const attrs = editor.getAttributes('image')
      if (attrs.src) {
        const a = document.createElement('a')
        a.href = attrs.src
        a.download = attrs.alt || 'image'
        a.target = '_blank'
        a.click()
      }
    },
    isActive: (_editor: Editor) => false,
  },
  'remove-image': {
    canExecute: (editor: Editor) => editor.isActive('image'),
    execute: (editor: Editor) => editor.chain().focus().deleteSelection().run(),
    isActive: (_editor: Editor) => false,
  },
}

// ── Draft & auto-save (E7) ────────────────────────────────────────────────
const draftFormData = ref<{ content?: string }>({ content: props.modelValue })
watch(() => props.modelValue, (val) => { draftFormData.value.content = val })

const {
  autoSavedLabel,
  showDraftRestore,
  savedDraft,
  hasUnsavedChanges,
  restoreDraft,
  discardDraft,
  markSaved,
  startAutoSave,
} = useEditorDraft(draftFormData, {
  mode: props.draftMode,
  entityId: props.draftEntityId,
  keyPrefix: props.draftKeyPrefix,
  hasInitialContent: props.hasInitialContent,
})

function onRestore() {
  restoreDraft()
  emit('update:modelValue', draftFormData.value.content ?? '')
}

// ── Word count + reading time (E7) ────────────────────────────────────────
const charCount = computed(() =>
  (props.modelValue ?? '')
    .replace(/[#*`[\]()>_~\-|]/g, '')
    .replace(/\s+/g, '')
    .trim().length,
)
const readingMinutes = computed(() => Math.max(1, Math.ceil(charCount.value / 400)))

onMounted(() => startAutoSave())

defineExpose({
  editor: computed(() => editorRef.value?.editor ?? null),
  markSaved,
  hasUnsavedChanges,
  showDraftRestore,
})
</script>

<template>
  <div>
    <!-- draft restore prompt -->
    <UAlert
      v-if="showDraftRestore"
      icon="i-tabler-device-floppy"
      color="warning"
      variant="subtle"
      title="发现未保存的本地草稿"
      class="mb-3">
      <template #description>
        <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <ClientOnly>
            <span class="text-sm text-muted">
              {{ savedDraft?.savedAt ? `保存于 ${new Date(savedDraft.savedAt).toLocaleString()}` : '' }}
            </span>
          </ClientOnly>
          <div class="flex gap-2">
            <UButton size="xs" color="primary" variant="soft" label="恢复草稿" @click="onRestore" />
            <UButton size="xs" color="neutral" variant="ghost" label="忽略" @click="discardDraft" />
          </div>
        </div>
      </template>
    </UAlert>

    <div class="rounded-xl border border-default">
    <UEditor
      ref="editorRef"
      v-slot="{ editor, handlers: editorHandlers }"
      :model-value="modelValue"
      content-type="markdown"
      :placeholder="placeholder"
      :extensions="editorExtensions"
      :starter-kit="{ codeBlock: false, blockquote: false }"
      :handlers="handlers"
      :ui="{ root: 'min-h-[480px]', content: 'prose prose-cyan max-w-none px-5 py-4 dark:prose-invert focus:outline-none' }"
      @update:model-value="emit('update:modelValue', $event)">
      <!-- toolbar (rounded-t to follow the frame; bg must not overflow the corner) -->
      <div class="sticky top-0 z-10 flex flex-wrap items-center gap-1 overflow-hidden rounded-t-xl border-b border-default bg-default/95 px-2 py-1.5 backdrop-blur sm:flex-nowrap sm:overflow-x-auto">
        <UEditorToolbar :editor="editor" :items="toolbarItems" layout="fixed" class="min-w-0 flex-1 flex-wrap sm:flex-nowrap" />
        <UIcon v-if="uploading" name="i-tabler-loader-2" class="ml-auto size-4 shrink-0 animate-spin text-muted" />
      </div>

      <!-- selection bubble -->
      <UEditorToolbar
        :editor="editor"
        :items="bubbleItems"
        class="z-50"
        layout="bubble"
        :should-show="({ editor: e, view, state }) => {
          const { selection } = state
          return view.hasFocus() && !selection.empty && !e.isActive('image')
        }" />

      <!-- image bubble -->
      <UEditorToolbar
        :editor="editor"
        :items="imageBubbleItems"
        class="z-50"
        layout="bubble"
        :should-show="({ editor: e }) => e.isActive('image')" />

      <!-- drag handle: “+” inserts via the suggestion menu, grip opens block actions -->
      <UEditorDragHandle
        v-slot="{ ui, onClick }"
        :editor="editor"
        @node-change="selectedNode = $event">
        <UButton
          icon="i-tabler-plus"
          color="neutral"
          variant="ghost"
          size="sm"
          :class="ui.handle()"
          @click="(e) => {
            e.stopPropagation()
            const sel = onClick()
            editorHandlers.suggestion?.execute(editor, { pos: sel?.pos }).run()
          }" />
        <UDropdownMenu
          v-slot="{ open }"
          :modal="false"
          :items="dragHandleItems(editor)"
          :content="{ side: 'left' }"
          :ui="{ content: 'w-52', label: 'text-xs' }"
          @update:open="editor.chain().setMeta('lockDragHandle', $event).run()">
          <UButton
            color="neutral"
            variant="ghost"
            active-variant="soft"
            size="sm"
            icon="i-tabler-grip-vertical"
            :active="open"
            :class="ui.handle()" />
        </UDropdownMenu>
      </UEditorDragHandle>

      <UEditorSuggestionMenu :editor="editor" :items="suggestionItems" :append-to="appendTo" />
      <UEditorEmojiMenu :editor="editor" :items="emojiItems" :append-to="appendTo" />
    </UEditor>

    <!-- word count -->
    <div class="flex items-center gap-4 border-t border-default px-4 py-2 text-xs text-muted">
      <span>{{ charCount }} 字</span>
      <span>约 {{ readingMinutes }} 分钟阅读</span>
      <ClientOnly>
        <span v-if="autoSavedLabel" class="ml-auto flex items-center gap-1">
          <UIcon name="i-tabler-circle-check" class="size-3 text-success" />{{ autoSavedLabel }}
        </span>
      </ClientOnly>
    </div>

    <input ref="fileInput" type="file" accept="image/*" class="hidden" @change="onFile">
    </div>
  </div>
</template>
