<script setup lang="ts">
/* eslint-disable vue/no-v-html */
import { NodeViewWrapper } from "@tiptap/vue-3";
import hljs from "highlight.js";
import { computed, nextTick, ref, watch } from "vue";
import { DEFAULT_CODE_LANGUAGES } from "../utils/codeLanguages";
import { useExclusiveNodeViewEditing } from "../utils/nodeViewEditing";

const props = defineProps<{
  node: {
    attrs: { language: string | null; code: string; editing?: boolean };
  };
  updateAttributes: (attrs: Record<string, unknown>) => void;
  selected: boolean;
}>();

const languages = DEFAULT_CODE_LANGUAGES;
const draft = ref(props.node.attrs.code || "");
const inputRef = ref<HTMLTextAreaElement>();
const { nodeViewId, activate } = useExclusiveNodeViewEditing({
  isEditing: () => Boolean(props.node.attrs.editing),
  close: finishEdit,
  focusSelector: "[data-editor-code-source]",
});

watch(
  () => props.node.attrs.code,
  (code) => {
    if (code !== draft.value) draft.value = code || "";
    if (!code && !props.node.attrs.editing) startEdit();
  },
  { immediate: true },
);

const highlighted = computed(() => {
  const code = draft.value;
  if (!code.trim()) return "";
  const lang = props.node.attrs.language;
  try {
    if (lang && hljs.getLanguage(lang)) {
      return hljs.highlight(code, { language: lang }).value;
    }
    return hljs.highlightAuto(code).value;
  } catch {
    return escapeHtml(code);
  }
});

function escapeHtml(text: string) {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function onLangChange(event: Event) {
  const value = (event.target as HTMLSelectElement).value;
  props.updateAttributes({ language: value || null });
}

function startEdit() {
  props.updateAttributes({ editing: true });
  activate();
  nextTick(() => {
    autoResize();
  });
}

function finishEdit() {
  props.updateAttributes({ editing: false });
}

function onInput(event: Event) {
  draft.value = (event.target as HTMLTextAreaElement).value;
  props.updateAttributes({ code: draft.value });
  autoResize();
}

function autoResize() {
  const element = inputRef.value;
  if (!element) return;
  element.style.height = "auto";
  element.style.height = `${element.scrollHeight}px`;
}

function onFocusOut(event: FocusEvent) {
  const root = event.currentTarget as HTMLElement;
  const next = event.relatedTarget as Node | null;
  if (next && root.contains(next)) return;
  requestAnimationFrame(() => {
    if (!root.contains(document.activeElement)) finishEdit();
  });
}

function onKeydown(event: KeyboardEvent) {
  if (event.key !== "Escape") return;
  finishEdit();
  event.preventDefault();
}
</script>

<template>
  <NodeViewWrapper
    data-editor-code-block
    :data-yueli-node-view-id="nodeViewId"
    class="relative my-[1em] overflow-hidden rounded-lg border bg-default transition-colors duration-150 focus-within:border-primary"
    :class="selected ? 'border-primary' : 'border-default'"
    @focusout="onFocusOut"
    @focusin="node.attrs.editing && activate()"
  >
    <div
      class="flex min-h-10 items-center justify-between gap-3 border-b border-default bg-elevated/60 px-3 py-1.5"
      contenteditable="false"
    >
      <div class="flex min-w-0 items-center gap-2">
        <span class="text-xs font-medium text-toned">代码</span>
        <select
          :value="node.attrs.language || ''"
          aria-label="代码语言"
          class="max-w-36 cursor-pointer rounded-md border border-default bg-default px-2 py-1 text-xs text-muted outline-none focus:border-primary"
          @change="onLangChange"
        >
          <option
            v-for="lang in languages"
            :key="lang.value"
            :value="lang.value"
          >
            {{ lang.label }}
          </option>
        </select>
      </div>
      <button
        type="button"
        class="rounded-md px-2.5 py-1 text-xs font-medium text-muted outline-none transition-colors hover:bg-accented hover:text-default focus-visible:ring-2 focus-visible:ring-primary"
        @mousedown.stop
        @click.stop="node.attrs.editing ? finishEdit() : startEdit()"
      >
        {{ node.attrs.editing ? "完成代码" : "编辑代码" }}
      </button>
    </div>

    <div :class="node.attrs.editing && 'grid md:grid-cols-2'">
      <section
        v-if="node.attrs.editing"
        class="min-w-0 border-b border-default bg-default md:border-b-0 md:border-e"
      >
        <div
          class="px-3 pt-2.5 text-[0.7rem] font-medium text-muted"
          contenteditable="false"
        >
          源码
        </div>
        <textarea
          ref="inputRef"
          data-editor-code-source
          :value="draft"
          aria-label="代码源码"
          class="min-h-32 w-full resize-none overflow-hidden border-0 bg-transparent px-3 pb-3 pt-1.5 font-mono text-[0.875em] leading-6 text-default outline-none"
          spellcheck="false"
          placeholder="输入代码…"
          @input="onInput"
          @keydown="onKeydown"
        />
      </section>

      <section
        data-editor-code-preview
        class="min-w-0 bg-muted/35 transition-colors"
        :class="
          node.attrs.editing
            ? 'min-h-32'
            : 'min-h-[3em] cursor-text hover:bg-muted/60'
        "
        contenteditable="false"
        @mousedown.stop
        @click.stop="!node.attrs.editing && startEdit()"
      >
        <div
          v-if="node.attrs.editing"
          class="px-3 pt-2.5 text-[0.7rem] font-medium text-muted"
        >
          预览
        </div>
        <div
          v-if="!draft.trim()"
          class="grid min-h-24 place-items-center p-3 text-sm text-muted"
        >
          点击输入代码…
        </div>
        <pre
          v-else
          class="!m-0 overflow-x-auto !rounded-none !border-0 !bg-transparent !px-3 !py-3 !shadow-none"
        ><code class="hljs font-mono text-[0.875em]" v-html="highlighted" /></pre>
      </section>
    </div>
  </NodeViewWrapper>
</template>
