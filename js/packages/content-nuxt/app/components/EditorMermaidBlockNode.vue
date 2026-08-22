<script setup lang="ts">
/* eslint-disable vue/no-v-html */
import { NodeViewWrapper } from "@tiptap/vue-3";
import { nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { renderMermaidSvg } from "../utils/mermaidRuntime";
import { useExclusiveNodeViewEditing } from "../utils/nodeViewEditing";

const props = defineProps<{
  node: {
    attrs: {
      code: string;
      editing?: boolean;
      previewSvg?: string;
      previewCode?: string;
      previewError?: string;
    };
  };
  updateAttributes: (attrs: Record<string, unknown>) => void;
  selected: boolean;
}>();

const inputRef = ref<HTMLTextAreaElement>();
const colorMode = useColorMode();
let destroyed = false;
const { nodeViewId, activate } = useExclusiveNodeViewEditing({
  isEditing: () => Boolean(props.node.attrs.editing),
  close: finishEdit,
  focusSelector: "[data-editor-mermaid-source]",
});

const previewMatchesCode = () =>
  Boolean(
    props.node.attrs.previewSvg &&
    props.node.attrs.previewCode === props.node.attrs.code,
  );

async function renderCurrentCode() {
  const code = props.node.attrs.code || "";
  const editing = Boolean(props.node.attrs.editing);
  if (!code.trim()) {
    props.updateAttributes({
      previewSvg: "",
      previewCode: "",
      previewError: "",
      editing,
    });
    return;
  }
  if (previewMatchesCode()) return;
  try {
    const id = `mermaid-editor-${Math.random().toString(36).slice(2, 8)}`;
    const { svg } = await renderMermaidSvg(
      id,
      code,
      colorMode.value === "dark" ? "dark" : "default",
    );
    if (destroyed) return;
    props.updateAttributes({
      previewSvg: svg,
      previewCode: code,
      previewError: "",
      editing,
    });
  } catch (error: unknown) {
    if (destroyed) return;
    props.updateAttributes({
      previewSvg: "",
      previewCode: "",
      previewError: error instanceof Error ? error.message : "渲染失败",
      editing,
    });
  }
}

onMounted(() => {
  if (!props.node.attrs.editing) void renderCurrentCode();
});
onBeforeUnmount(() => {
  destroyed = true;
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
  setTimeout(() => {
    if (!destroyed) void renderCurrentCode();
  }, 0);
}

function onInput(event: Event) {
  const value = (event.target as HTMLTextAreaElement).value;
  props.updateAttributes({ code: value });
  autoResize();
}

async function renderDraftPreview(event: MouseEvent) {
  const button = event.currentTarget as HTMLButtonElement;
  const root = button.closest<HTMLElement>("[data-editor-mermaid-block]");
  const input = root?.querySelector<HTMLTextAreaElement>(
    "[data-editor-mermaid-source]",
  );
  const output = root?.querySelector<HTMLElement>(
    "[data-editor-mermaid-live-output]",
  );
  const prompt = root?.querySelector<HTMLElement>(
    "[data-editor-mermaid-live-prompt]",
  );
  const code = input?.value || "";
  if (!root || !output || !prompt || !code.trim()) return;
  if (button.dataset.loading === "true") return;

  button.dataset.loading = "true";
  button.setAttribute("aria-busy", "true");
  button.textContent = "生成中…";
  try {
    const id = `mermaid-editor-${Math.random().toString(36).slice(2, 8)}`;
    const { svg } = await renderMermaidSvg(
      id,
      code,
      colorMode.value === "dark" ? "dark" : "default",
    );
    if (!root.isConnected || input?.value !== code) return;
    output.innerHTML = svg;
    output.hidden = false;
    input.focus();
    prompt.hidden = true;
  } catch (error: unknown) {
    output.textContent =
      error instanceof Error ? error.message : "预览生成失败";
    output.classList.add("text-error", "text-sm");
    output.hidden = false;
  } finally {
    delete button.dataset.loading;
    button.removeAttribute("aria-busy");
    button.textContent = "更新预览";
  }
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
  if (next) {
    finishEdit();
    return;
  }
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
    data-editor-mermaid-block
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
      <span class="text-xs font-medium text-toned">Mermaid</span>
      <button
        type="button"
        class="rounded-md px-2.5 py-1 text-xs font-medium text-muted outline-none transition-colors hover:bg-accented hover:text-default focus-visible:ring-2 focus-visible:ring-primary"
        @mousedown.stop
        @click.stop="node.attrs.editing ? finishEdit() : startEdit()"
      >
        {{ node.attrs.editing ? "完成图表" : "编辑图表" }}
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
          data-editor-mermaid-source
          :value="node.attrs.code"
          aria-label="Mermaid 源码"
          class="min-h-36 w-full resize-none overflow-hidden border-0 bg-transparent px-3 pb-3 pt-1.5 font-mono text-[0.875em] leading-6 text-default outline-none"
          spellcheck="false"
          placeholder="graph TD&#10;    A--&gt;B"
          @input="onInput"
          @keydown="onKeydown"
        />
      </section>

      <section
        data-editor-mermaid-preview
        class="min-w-0 overflow-x-auto bg-muted/35 p-3 text-center transition-colors"
        :class="
          node.attrs.editing
            ? 'min-h-36'
            : 'grid min-h-28 cursor-text place-items-center hover:bg-muted/60'
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
        <div
          v-if="!node.attrs.code?.trim()"
          class="grid min-h-20 place-items-center text-sm text-muted"
        >
          点击输入 Mermaid 图表代码
        </div>
        <template v-else-if="node.attrs.editing">
          <div
            data-editor-mermaid-live-output
            :hidden="!previewMatchesCode()"
            v-html="previewMatchesCode() ? node.attrs.previewSvg : ''"
          />
          <div
            data-editor-mermaid-live-prompt
            :hidden="previewMatchesCode()"
            class="grid min-h-20 place-items-center gap-2 text-sm text-muted"
          >
            <span>源码已修改</span>
            <button
              type="button"
              class="rounded-lg border border-default bg-default px-3 py-1.5 text-xs font-medium text-toned outline-none hover:bg-accented hover:text-default focus-visible:ring-2 focus-visible:ring-primary aria-busy:cursor-wait aria-busy:opacity-60"
              @click.stop="renderDraftPreview"
            >
              更新预览
            </button>
          </div>
        </template>
        <div v-else-if="node.attrs.previewError" class="text-left">
          <div class="mb-2 text-sm text-error">
            {{ node.attrs.previewError }}
          </div>
          <pre
            class="!m-0 whitespace-pre-wrap break-all !rounded-none !border-0 !bg-transparent !p-0 text-xs text-muted !shadow-none"
            >{{ node.attrs.code }}</pre>
        </div>
        <div v-else-if="node.attrs.previewSvg" v-html="node.attrs.previewSvg" />
        <div v-else class="grid min-h-20 place-items-center text-sm text-muted">
          正在生成预览…
        </div>
      </section>
    </div>
  </NodeViewWrapper>
</template>
