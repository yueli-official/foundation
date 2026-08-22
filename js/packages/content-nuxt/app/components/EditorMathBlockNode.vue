<script setup lang="ts">
/* eslint-disable vue/no-v-html */
import { NodeViewWrapper } from "@tiptap/vue-3";
import katex from "katex";
import { computed, nextTick, ref, watch } from "vue";
import { useExclusiveNodeViewEditing } from "../utils/nodeViewEditing";

const props = defineProps<{
  node: { attrs: { latex: string; editing?: boolean } };
  updateAttributes: (attrs: Record<string, unknown>) => void;
  selected: boolean;
}>();

const draft = ref(props.node.attrs.latex || "");
const inputRef = ref<HTMLTextAreaElement>();
const { nodeViewId, activate } = useExclusiveNodeViewEditing({
  isEditing: () => Boolean(props.node.attrs.editing),
  close: finishEdit,
  focusSelector: "[data-editor-math-source]",
});

watch(
  () => props.node.attrs.latex,
  (latex) => {
    if (latex !== draft.value) draft.value = latex || "";
  },
);

const rendered = computed(() => {
  if (!draft.value.trim()) {
    return '<span class="text-muted text-sm">点击输入公式</span>';
  }
  try {
    return katex.renderToString(draft.value, {
      displayMode: true,
      throwOnError: false,
    });
  } catch {
    return `<span class="text-error text-sm">${draft.value}</span>`;
  }
});

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
  props.updateAttributes({ latex: draft.value });
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
    data-editor-math-block
    :data-yueli-node-view-id="nodeViewId"
    class="my-[1em] overflow-hidden rounded-lg border bg-default transition-colors duration-150 focus-within:border-primary"
    :class="selected ? 'border-primary' : 'border-default'"
    @focusout="onFocusOut"
    @focusin="node.attrs.editing && activate()"
  >
    <div
      class="flex min-h-10 items-center justify-between gap-3 border-b border-default bg-elevated/60 px-3 py-1.5"
      contenteditable="false"
    >
      <span class="text-xs font-medium text-toned">公式</span>
      <button
        type="button"
        class="rounded-md px-2.5 py-1 text-xs font-medium text-muted outline-none transition-colors hover:bg-accented hover:text-default focus-visible:ring-2 focus-visible:ring-primary"
        @mousedown.stop
        @click.stop="node.attrs.editing ? finishEdit() : startEdit()"
      >
        {{ node.attrs.editing ? "完成公式" : "编辑公式" }}
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
          LaTeX 源码
        </div>
        <textarea
          ref="inputRef"
          data-editor-math-source
          :value="draft"
          aria-label="LaTeX 源码"
          class="min-h-32 w-full resize-none overflow-hidden border-0 bg-transparent px-3 pb-3 pt-1.5 font-mono text-[0.875em] leading-6 text-default outline-none"
          spellcheck="false"
          placeholder="E = mc^2"
          @input="onInput"
          @keydown="onKeydown"
        />
      </section>

      <section
        data-editor-math-preview
        class="min-w-0 overflow-x-auto bg-muted/35 px-4 py-3 text-center transition-colors"
        :class="
          node.attrs.editing
            ? 'min-h-32'
            : 'grid min-h-[4.5rem] cursor-text place-items-center hover:bg-muted/60'
        "
        contenteditable="false"
        @mousedown.stop
        @click.stop="!node.attrs.editing && startEdit()"
      >
        <div
          v-if="node.attrs.editing"
          class="mb-2 text-left text-[0.7rem] font-medium text-muted"
        >
          预览
        </div>
        <div v-html="rendered" />
      </section>
    </div>
  </NodeViewWrapper>
</template>
