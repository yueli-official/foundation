<script setup lang="ts">
/* eslint-disable vue/no-v-html */
import { NodeViewWrapper } from "@tiptap/vue-3";
import hljs from "highlight.js";
import { DEFAULT_CODE_LANGUAGES } from "../utils/codeLanguages";

// 代码块 NodeView 提供语言选择、点击编辑和 highlight.js 预览；
// token 颜色来自 article.css，不依赖 CDN。
const props = defineProps<{
  node: { attrs: { language: string | null; code: string } };
  updateAttributes: (attrs: Record<string, unknown>) => void;
  selected: boolean;
}>();

const languages = DEFAULT_CODE_LANGUAGES;

const editing = ref(false);
const inputRef = ref<HTMLTextAreaElement>();

// 空代码块自动进入编辑状态。
watch(
  () => props.node.attrs.code,
  (code) => {
    if (!code && !editing.value) {
      startEdit();
    }
  },
  { immediate: true },
);

const highlighted = computed(() => {
  const code = props.node.attrs.code || "";
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
  const val = (event.target as HTMLSelectElement).value;
  props.updateAttributes({ language: val || null });
}

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
    class="relative my-[1em] overflow-hidden rounded-xl border transition-colors duration-150 focus-within:border-primary"
    :class="selected ? 'border-primary' : 'border-default'"
  >
    <div
      class="flex justify-end border-b border-default bg-elevated px-2 py-1"
      contenteditable="false"
    >
      <select
        :value="node.attrs.language || ''"
        class="cursor-pointer rounded border border-default bg-default px-1.5 py-0.5 text-xs text-muted outline-none focus:border-primary"
        @change="onLangChange"
      >
        <option v-for="lang in languages" :key="lang.value" :value="lang.value">
          {{ lang.label }}
        </option>
      </select>
    </div>
    <!-- 编辑状态 -->
    <div v-if="editing" class="bg-default">
      <textarea
        ref="inputRef"
        :value="node.attrs.code"
        class="min-h-[3em] w-full resize-none overflow-hidden border-0 bg-transparent px-[1em] py-[0.75em] font-mono text-[0.875em] leading-6 text-default outline-none"
        spellcheck="false"
        placeholder="点击输入代码…"
        @input="onInput"
        @blur="onBlur"
        @keydown="onKeydown"
      />
    </div>
    <!-- 预览状态；highlight.js 输出会转义原始代码。 -->
    <div
      v-else
      class="min-h-[3em] cursor-pointer hover:bg-elevated"
      contenteditable="false"
      @click="startEdit()"
    >
      <div
        v-if="!node.attrs.code?.trim()"
        class="p-[1em] text-center text-sm text-muted"
      >
        点击输入代码…
      </div>
      <pre
        v-else
        class="m-0 overflow-x-auto rounded-none border-0 bg-transparent p-[1em]"
      ><code class="hljs font-mono text-[0.875em]" v-html="highlighted" /></pre>
    </div>
  </NodeViewWrapper>
</template>
