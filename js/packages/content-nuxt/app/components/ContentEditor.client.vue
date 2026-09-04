<script setup lang="ts">
import type { Editor } from "@tiptap/vue-3";
import { Emoji, gitHubEmojis } from "@tiptap/extension-emoji";
import { TextAlign } from "@tiptap/extension-text-align";
import Blockquote from "@tiptap/extension-blockquote";
import { Callout, type CalloutType } from "../extensions/Callout";
import { CodeBlockWithLang } from "../extensions/CodeBlockWithLang";
import { EditableInlineMath } from "../extensions/EditableInlineMath";
import { MathBlock } from "../extensions/MathBlock";
import { MermaidBlock } from "../extensions/MermaidBlock";

interface ChainBoundary {
  deleteSelection(): ChainBoundary;
  focus(): ChainBoundary;
  insertContent(content: unknown): ChainBoundary;
  run(): boolean;
}

interface EditorBoundary {
  chain(): ChainBoundary;
  getAttributes(name: string): { alt?: string; src?: string };
  isActive(name: string, attributes?: Record<string, unknown>): boolean;
  isEditable: boolean;
}

// Markdown 富文本编辑器与 ContentProse 共用文档语义。代码、公式、Mermaid、
// 提示块、草稿、表情和字数统计都封装在本基础库内，产品只负责保存内容和上传图片。
const props = withDefaults(
  defineProps<{
    modelValue: string;
    placeholder?: string;
    // 产品注入图片上传能力并返回公开 URL；未提供时图片入口不可用。
    imageUploader?: (file: File) => Promise<string>;
    // 草稿按实体隔离；已有服务端内容时不得被陈旧本地草稿静默覆盖。
    draftEntityId?: string | number;
    draftMode?: "create" | "edit";
    hasInitialContent?: boolean;
    draftEnabled?: boolean;
    // 各产品使用独立命名空间，避免不同内容类型的本地草稿互相覆盖。
    draftKeyPrefix?: string;
    // 产品有独立文档标题时可从 H2 开始，其他消费者仍保留完整层级。
    allowHeadingOne?: boolean;
  }>(),
  {
    placeholder: "开始写作…支持 Markdown 与富文本",
    imageUploader: undefined,
    draftEntityId: undefined,
    draftMode: "edit",
    hasInitialContent: false,
    draftEnabled: true,
    draftKeyPrefix: "content:entry",
    allowHeadingOne: true,
  },
);

const emit = defineEmits<{ "update:modelValue": [value: string] }>();

const toast = useToast();

// 自定义节点替换 starter-kit 的 codeBlock 和 blockquote。
const editorExtensions = [
  Emoji,
  TextAlign.configure({ types: ["heading", "paragraph"] }),
  // Callout 接管 blockquote token。Tiptap 运行时以 null 覆盖继承的解析器，
  // 但类型声明尚未暴露这个哨兵值，因此在此收窄为 never。
  Blockquote.extend({ parseMarkdown: null as never }),
  EditableInlineMath,
  MathBlock,
  MermaidBlock,
  Callout,
  CodeBlockWithLang,
];

const appendTo = import.meta.client ? () => document.body : undefined;
const emojiItems = gitHubEmojis.filter(
  (e) => !e.name.startsWith("regional_indicator_"),
);

// 工具栏配置。
const {
  toolbarItems,
  bubbleItems,
  suggestionItems,
  imageBubbleItems,
  selectedNode,
  dragHandleItems,
} = useEditorToolbar({ allowHeadingOne: props.allowHeadingOne });

// 图片入口供工具栏和建议菜单共用。
const editorRef = ref<{ editor?: Editor } | null>(null);
const fileInput = ref<HTMLInputElement>();
const uploading = ref(false);

function pickImage() {
  if (!props.imageUploader) return;
  fileInput.value?.click();
}

async function onFile(e: Event) {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (file) await insertImageFile(file);
}

async function insertImageFile(file: File, position?: number) {
  // 始终从 UEditor 暴露值读取实例，保证工具栏、粘贴和拖放共用同一上传 seam。
  const editor = editorRef.value?.editor;
  if (!editor || !props.imageUploader || uploading.value) return;
  uploading.value = true;
  try {
    if (position !== undefined) {
      editor.chain().focus().setTextSelection(position).run();
    }
    const url = await props.imageUploader(file);
    editor.chain().focus().setImage({ src: url, alt: file.name }).run();
  } catch (error: unknown) {
    toast.add({
      title: "图片上传失败",
      description: error instanceof Error ? error.message : "请重试",
      color: "error",
    });
  } finally {
    uploading.value = false;
  }
}

function firstImage(files: FileList | null | undefined) {
  return Array.from(files || []).find((file) => file.type.startsWith("image/"));
}

function onPaste(event: ClipboardEvent) {
  const file = firstImage(event.clipboardData?.files);
  if (!file || !props.imageUploader) return;
  event.preventDefault();
  void insertImageFile(file);
}

function onDrop(event: DragEvent) {
  const file = firstImage(event.dataTransfer?.files);
  const editor = editorRef.value?.editor;
  if (!file || !editor || !props.imageUploader) return;
  event.preventDefault();
  const position = editor.view.posAtCoords({
    left: event.clientX,
    top: event.clientY,
  })?.pos;
  void insertImageFile(file, position);
}

function onDragOver(event: DragEvent) {
  if (
    props.imageUploader &&
    Array.from(event.dataTransfer?.types || []).includes("Files")
  ) {
    event.preventDefault();
  }
}

// 工具栏、气泡菜单和建议菜单共用命令处理器。
const calloutHandler = (type: CalloutType) => ({
  canExecute: (editor: EditorBoundary) => editor.isEditable,
  execute: (editor: EditorBoundary) =>
    (editor as unknown as Editor).chain().focus().setCallout({ type }),
  isActive: (editor: EditorBoundary) => editor.isActive("callout", { type }),
});

const handlers = {
  image: {
    canExecute: (editor: EditorBoundary) =>
      editor.isEditable && !!props.imageUploader,
    execute: (editor: EditorBoundary) => {
      pickImage();
      return editor.chain();
    },
    isActive: (_editor: EditorBoundary) => false,
  },
  "callout-note": calloutHandler("note"),
  "callout-tip": calloutHandler("tip"),
  "callout-important": calloutHandler("important"),
  "callout-warning": calloutHandler("warning"),
  "callout-caution": calloutHandler("caution"),
  "math-inline": {
    canExecute: (editor: EditorBoundary) => editor.isEditable,
    execute: (editor: EditorBoundary) =>
      editor
        .chain()
        .focus()
        .insertContent({
          type: "inlineMath",
          attrs: { latex: "E = mc^2", editing: true },
        }),
    isActive: (_editor: EditorBoundary) => false,
  },
  "math-block": {
    canExecute: (editor: EditorBoundary) => editor.isEditable,
    execute: (editor: EditorBoundary) =>
      (editor as unknown as Editor)
        .chain()
        .focus()
        .setMathBlock({ latex: "E = mc^2", editing: true }),
    isActive: (editor: EditorBoundary) => editor.isActive("blockMath"),
  },
  "mermaid-block": {
    canExecute: (editor: EditorBoundary) => editor.isEditable,
    execute: (editor: EditorBoundary) =>
      (editor as unknown as Editor)
        .chain()
        .focus()
        .setMermaidBlock({ code: "graph TD\n    A-->B", editing: true }),
    isActive: (editor: EditorBoundary) => editor.isActive("mermaidBlock"),
  },
  "download-image": {
    canExecute: (editor: EditorBoundary) => editor.isActive("image"),
    execute: (editor: EditorBoundary) => {
      const attrs = editor.getAttributes("image");
      if (attrs.src) {
        const a = document.createElement("a");
        a.href = attrs.src;
        a.download = attrs.alt || "image";
        a.target = "_blank";
        a.click();
      }
      return editor.chain();
    },
    isActive: (_editor: EditorBoundary) => false,
  },
  "remove-image": {
    canExecute: (editor: EditorBoundary) => editor.isActive("image"),
    execute: (editor: EditorBoundary) =>
      editor.chain().focus().deleteSelection(),
    isActive: (_editor: EditorBoundary) => false,
  },
};

// Tiptap classes contain private fields, so source-layer consumers using a
// different compatible patch see distinct nominal types even though Vite
// resolves and deduplicates them to one runtime instance. Erase only at the
// Nuxt UI prop boundary; node implementations retain their full types.
const uiEditorExtensions = editorExtensions as never;
const uiEditorHandlers = handlers;

// 草稿与自动保存。
const draftFormData = ref<{ content?: string }>({ content: props.modelValue });
watch(
  () => props.modelValue,
  (val) => {
    draftFormData.value.content = val;
  },
);

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
  entityId: () => props.draftEntityId,
  keyPrefix: props.draftKeyPrefix,
  hasInitialContent: props.hasInitialContent,
});

function onRestore() {
  const restored = restoreDraft();
  if (restored) emit("update:modelValue", restored.content ?? "");
}

// 字数与预计阅读时间。
const charCount = computed(
  () =>
    (props.modelValue ?? "")
      .replace(/[#*`[\]()>_~\-|]/g, "")
      .replace(/\s+/g, "")
      .trim().length,
);
const readingMinutes = computed(() =>
  Math.max(1, Math.ceil(charCount.value / 400)),
);

onMounted(() => {
  if (props.draftEnabled) startAutoSave();
});

defineExpose({
  editor: computed(() => editorRef.value?.editor ?? null),
  markSaved,
  hasUnsavedChanges,
  showDraftRestore,
});
</script>

<template>
  <div
    @paste.capture="onPaste"
    @dragover.capture="onDragOver"
    @drop.capture="onDrop"
  >
    <!-- draft restore prompt -->
    <UAlert
      v-if="showDraftRestore"
      icon="i-tabler-device-floppy"
      color="warning"
      variant="subtle"
      title="发现未保存的本地草稿"
      class="mb-3"
    >
      <template #description>
        <div
          class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between"
        >
          <ClientOnly>
            <span class="text-sm text-muted">
              {{
                savedDraft?.savedAt
                  ? `保存于 ${new Date(savedDraft.savedAt).toLocaleString()}`
                  : ""
              }}
            </span>
          </ClientOnly>
          <div class="flex gap-2">
            <UButton
              size="xs"
              color="primary"
              variant="soft"
              label="恢复草稿"
              @click="onRestore"
            />
            <UButton
              size="xs"
              color="neutral"
              variant="ghost"
              label="忽略"
              @click="discardDraft"
            />
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
        :extensions="uiEditorExtensions"
        :starter-kit="{ codeBlock: false, blockquote: false }"
        :handlers="uiEditorHandlers"
        :ui="{
          root: 'min-h-[480px]',
          content:
            'prose prose-cyan max-w-none px-5 py-4 dark:prose-invert focus:outline-none',
        }"
        @update:model-value="emit('update:modelValue', $event)"
      >
        <!-- toolbar (rounded-t to follow the frame; bg must not overflow the corner) -->
        <div
          class="sticky z-20 flex flex-nowrap items-center gap-1 overflow-x-auto rounded-t-xl border-b border-default bg-default/95 px-2 py-1.5 backdrop-blur"
          style="top: var(--content-editor-toolbar-top, 0px)"
          data-content-editor-toolbar
        >
          <UEditorToolbar
            :editor="editor"
            :items="toolbarItems"
            layout="fixed"
            class="min-w-max flex-none [&_[role=group]_button]:min-h-11 [&_[role=group]_button]:min-w-11 sm:[&_[role=group]_button]:min-h-8 sm:[&_[role=group]_button]:min-w-8"
          />
          <UIcon
            v-if="uploading"
            name="i-tabler-loader-2"
            class="ml-auto size-4 shrink-0 animate-spin text-muted"
          />
          <div
            v-if="$slots['toolbar-actions']"
            class="ml-auto flex shrink-0 items-center gap-1"
          >
            <slot name="toolbar-actions" />
          </div>
        </div>

        <!-- selection bubble -->
        <UEditorToolbar
          :editor="editor"
          :items="bubbleItems"
          class="z-50"
          layout="bubble"
          :should-show="
            ({ editor: e, view, state }) => {
              const { selection } = state;
              return (
                view.hasFocus() && !selection.empty && !e.isActive('image')
              );
            }
          "
        />

        <!-- image bubble -->
        <UEditorToolbar
          :editor="editor"
          :items="imageBubbleItems"
          class="z-50"
          layout="bubble"
          :should-show="({ editor: e }) => e.isActive('image')"
        />

        <!-- drag handle: “+” inserts via the suggestion menu, grip opens block actions -->
        <UEditorDragHandle
          v-slot="{ ui, onClick }"
          :editor="editor"
          @node-change="selectedNode = $event"
        >
          <UButton
            icon="i-tabler-plus"
            color="neutral"
            variant="ghost"
            size="sm"
            :class="ui.handle()"
            @click="
              (e) => {
                e.stopPropagation();
                const sel = onClick();
                editorHandlers.suggestion
                  ?.execute(editor, { pos: sel?.pos })
                  .run();
              }
            "
          />
          <UDropdownMenu
            v-slot="{ open }"
            :modal="false"
            :items="dragHandleItems(editor)"
            :content="{ side: 'left' }"
            :ui="{ content: 'w-52', label: 'text-xs' }"
            @update:open="
              editor.chain().setMeta('lockDragHandle', $event).run()
            "
          >
            <UButton
              color="neutral"
              variant="ghost"
              active-variant="soft"
              size="sm"
              icon="i-tabler-grip-vertical"
              :active="open"
              :class="ui.handle()"
            />
          </UDropdownMenu>
        </UEditorDragHandle>

        <UEditorSuggestionMenu
          :editor="editor"
          :items="suggestionItems"
          :append-to="appendTo"
        />
        <UEditorEmojiMenu
          :editor="editor"
          :items="emojiItems"
          :append-to="appendTo"
        />
      </UEditor>

      <!-- word count -->
      <div
        class="flex items-center gap-4 border-t border-default px-4 py-2 text-xs text-muted"
      >
        <span>{{ charCount }} 字</span>
        <span>约 {{ readingMinutes }} 分钟阅读</span>
        <ClientOnly>
          <span v-if="autoSavedLabel" class="ml-auto flex items-center gap-1">
            <UIcon name="i-tabler-circle-check" class="size-3 text-success" />{{
              autoSavedLabel
            }}
          </span>
        </ClientOnly>
      </div>

      <input
        ref="fileInput"
        type="file"
        accept="image/*"
        class="hidden"
        @change="onFile"
      />
    </div>
  </div>
</template>
