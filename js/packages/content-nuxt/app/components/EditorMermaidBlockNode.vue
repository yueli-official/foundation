<script setup lang="ts">
/* eslint-disable vue/no-v-html */
import { NodeViewWrapper } from "@tiptap/vue-3";

// Mermaid NodeView 支持点击编辑和延迟实时预览；Mermaid 按需加载。
const props = defineProps<{
  node: { attrs: { code: string } };
  updateAttributes: (attrs: Record<string, unknown>) => void;
  selected: boolean;
}>();

const editing = ref(false);
const inputRef = ref<HTMLTextAreaElement>();
const svgHtml = ref("");
const renderError = ref("");

const colorMode = useColorMode();

// 模块只加载一次，但每次渲染重新应用主题以跟随当前配色。
let mermaidInstance: (typeof import("mermaid"))["default"] | null = null;

async function getMermaid() {
  if (!mermaidInstance) {
    const mod = await import("mermaid");
    mermaidInstance = mod.default;
  }
  mermaidInstance.initialize({
    startOnLoad: false,
    theme: colorMode.value === "dark" ? "dark" : "default",
  });
  return mermaidInstance;
}

// 输入停止后再渲染，避免每个按键都重建 SVG。
let renderTimer: ReturnType<typeof setTimeout> | null = null;

async function renderMermaid(code: string) {
  if (!code.trim()) {
    svgHtml.value = "";
    renderError.value = "";
    return;
  }
  try {
    const mermaid = await getMermaid();
    const id = `mermaid-editor-${Math.random().toString(36).slice(2, 8)}`;
    const { svg } = await mermaid.render(id, code);
    svgHtml.value = svg;
    renderError.value = "";
  } catch (error: unknown) {
    renderError.value = error instanceof Error ? error.message : "渲染失败";
  }
}

watch(
  () => props.node.attrs.code,
  (code) => {
    if (renderTimer) clearTimeout(renderTimer);
    renderTimer = setTimeout(() => renderMermaid(code), 500);
  },
  { immediate: true },
);

// 明暗主题切换后重新渲染。
watch(
  () => colorMode.value,
  () => renderMermaid(props.node.attrs.code),
);

function startEdit() {
  editing.value = true;
  nextTick(() => {
    inputRef.value?.focus();
    autoResize();
  });
}

function onInput(e: Event) {
  const val = (e.target as HTMLTextAreaElement).value;
  props.updateAttributes({ code: val });
  autoResize();
}

function autoResize() {
  const el = inputRef.value;
  if (!el) return;
  el.style.height = "auto";
  el.style.height = el.scrollHeight + "px";
}

function onBlur() {
  editing.value = false;
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") {
    editing.value = false;
    e.preventDefault();
  }
}
</script>

<template>
  <NodeViewWrapper
    class="my-[1em] overflow-hidden rounded-xl border transition-colors duration-150 focus-within:border-primary"
    :class="selected ? 'border-primary' : 'border-default'"
  >
    <!-- Edit area -->
    <div v-if="editing" class="border-b border-default">
      <div
        class="flex items-center border-b border-default bg-elevated px-3 py-1"
        contenteditable="false"
      >
        <span
          class="text-[0.7rem] font-semibold uppercase tracking-[0.05em] text-muted"
          >Mermaid</span
        >
      </div>
      <textarea
        ref="inputRef"
        :value="node.attrs.code"
        class="min-h-[4em] w-full resize-none overflow-hidden border-0 bg-default p-[0.75em] font-mono text-[0.875em] leading-6 text-default outline-none"
        spellcheck="false"
        placeholder="graph TD&#10;    A--&gt;B"
        @input="onInput"
        @blur="onBlur"
        @keydown="onKeydown"
      />
    </div>
    <!-- Preview -->
    <div
      class="overflow-x-auto p-[1em] text-center"
      :class="{
        'flex min-h-[3em] cursor-pointer items-center justify-center hover:bg-elevated':
          !editing,
      }"
      contenteditable="false"
      @click="!editing && startEdit()"
    >
      <!-- Empty placeholder -->
      <div
        v-if="!node.attrs.code?.trim() && !editing"
        class="text-muted text-sm"
      >
        点击输入 Mermaid 图表代码
      </div>
      <!-- Error -->
      <div v-else-if="renderError" class="p-[0.5em] text-left">
        <div class="text-error text-sm mb-2">{{ renderError }}</div>
        <pre class="whitespace-pre-wrap break-all text-xs text-muted">{{
          node.attrs.code
        }}</pre>
      </div>
      <!-- SVG -->
      <div v-else-if="svgHtml" v-html="svgHtml" />
      <!-- Loading -->
      <div v-else class="text-muted text-sm">…</div>
    </div>
  </NodeViewWrapper>
</template>
