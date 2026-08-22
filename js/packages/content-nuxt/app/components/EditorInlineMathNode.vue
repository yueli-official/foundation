<script setup lang="ts">
/* eslint-disable vue/no-v-html */
import { NodeViewWrapper } from "@tiptap/vue-3";
import katex from "katex";
import { computed, nextTick, ref, watch } from "vue";
import { useExclusiveNodeViewEditing } from "../utils/nodeViewEditing";

const props = defineProps<{
  node: { attrs: { latex: string; editing?: boolean } };
  updateAttributes: (attrs: Record<string, unknown>) => void;
}>();

const draft = ref(props.node.attrs.latex || "");
const inputRef = ref<HTMLInputElement>();
const { nodeViewId, activate } = useExclusiveNodeViewEditing({
  isEditing: () => Boolean(props.node.attrs.editing),
  close: finishEdit,
  focusSelector: "[data-editor-inline-math-source]",
});

watch(
  () => props.node.attrs.latex,
  (latex) => {
    if (latex !== draft.value) draft.value = latex || "";
  },
);

const rendered = computed(() =>
  katex.renderToString(draft.value || "\\square", {
    displayMode: false,
    throwOnError: false,
  }),
);

function startEdit() {
  props.updateAttributes({ editing: true });
  activate();
  nextTick(() => {
    inputRef.value?.select();
  });
}

function finishEdit() {
  props.updateAttributes({ editing: false });
}

function onInput(event: Event) {
  draft.value = (event.target as HTMLInputElement).value;
  props.updateAttributes({ latex: draft.value });
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
  if (event.key !== "Escape" && event.key !== "Enter") return;
  finishEdit();
  event.preventDefault();
}
</script>

<template>
  <NodeViewWrapper
    as="span"
    data-editor-inline-math
    :data-yueli-node-view-id="nodeViewId"
    data-type="inline-math"
    :data-latex="draft"
    class="relative z-[1] inline-flex align-middle"
    contenteditable="false"
    @focusout="onFocusOut"
    @focusin="node.attrs.editing && activate()"
  >
    <button
      type="button"
      class="inline-flex cursor-text items-center rounded px-1 py-0.5 text-default outline-none transition-colors hover:bg-primary/10 focus-visible:bg-primary/10 focus-visible:ring-2 focus-visible:ring-primary"
      :aria-label="`编辑行内公式：${draft}`"
      :aria-expanded="Boolean(node.attrs.editing)"
      @mousedown.stop
      @click.stop="!node.attrs.editing && startEdit()"
    >
      <span v-html="rendered" />
    </button>

    <span
      v-if="node.attrs.editing"
      class="absolute start-0 top-full z-50 mt-2 w-80 max-w-[min(20rem,calc(100vw-2rem))] rounded-xl border border-default bg-default p-3 text-start shadow-lg"
      @mousedown.stop
      @click.stop
    >
      <span class="mb-2 flex items-center justify-between gap-3">
        <span class="text-xs font-medium text-toned">LaTeX</span>
        <button
          type="button"
          class="rounded-md px-2 py-1 text-xs font-medium text-muted outline-none hover:bg-muted hover:text-default focus-visible:ring-2 focus-visible:ring-primary"
          @click.stop="finishEdit"
        >
          完成
        </button>
      </span>
      <input
        ref="inputRef"
        data-editor-inline-math-source
        :value="draft"
        aria-label="行内公式 LaTeX"
        class="block w-full rounded-lg border border-default bg-muted px-2.5 py-2 font-mono text-sm text-default outline-none focus:border-primary"
        spellcheck="false"
        @input="onInput"
        @keydown="onKeydown"
      />
      <span
        data-editor-inline-math-preview
        class="mt-2 grid min-h-12 place-items-center overflow-x-auto rounded-lg bg-muted/60 px-3 py-2 text-center"
      >
        <span v-html="rendered" />
      </span>
    </span>
  </NodeViewWrapper>
</template>
