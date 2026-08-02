<script setup lang="ts">
import { NodeViewWrapper, NodeViewContent } from "@tiptap/vue-3";
import type { CalloutType } from "../extensions/Callout";

// Editor-side NodeView for Callout (E6): a type chip (click to cycle) + editable
// body content. Ported from donor.
const props = defineProps<{
  node: { attrs: { type: CalloutType } };
  updateAttributes: (attrs: Record<string, unknown>) => void;
}>();

const types: { value: CalloutType; label: string; icon: string }[] = [
  { value: "note", label: "提示", icon: "i-tabler-info-circle" },
  { value: "tip", label: "建议", icon: "i-tabler-bulb" },
  { value: "important", label: "重要", icon: "i-tabler-alert-circle" },
  { value: "warning", label: "警告", icon: "i-tabler-alert-triangle" },
  { value: "caution", label: "危险", icon: "i-tabler-flame" },
];

const currentType = computed(
  () => types.find((t) => t.value === props.node.attrs.type) || types[0]!,
);

const colorMap: Record<CalloutType, string> = {
  note: "var(--ui-primary)",
  tip: "var(--ui-success)",
  important: "#a855f7",
  warning: "var(--ui-warning)",
  caution: "var(--ui-error)",
};

function cycleType() {
  const idx = types.findIndex((t) => t.value === props.node.attrs.type);
  const next = types[(idx + 1) % types.length]!;
  props.updateAttributes({ type: next.value });
}
</script>

<template>
  <NodeViewWrapper
    class="my-[1em] rounded-r-lg border-l-4 px-[1.25em] py-[0.75em]"
    :style="{
      borderLeftColor: colorMap[node.attrs.type],
      background: `color-mix(in srgb, ${colorMap[node.attrs.type]} 6%, transparent)`,
    }"
  >
    <div
      class="mb-[0.25em] flex select-none items-center gap-[0.5em]"
      contenteditable="false"
    >
      <button
        class="inline-flex cursor-pointer items-center gap-[0.25em] rounded border-0 bg-transparent px-[0.375em] py-[0.125em] transition-colors duration-150 hover:bg-current/10"
        :style="{ color: colorMap[node.attrs.type] }"
        :title="`切换类型(当前:${currentType.label})`"
        @click="cycleType"
      >
        <UIcon :name="currentType.icon" class="size-4" />
        <span class="text-xs font-semibold uppercase">{{
          currentType.label
        }}</span>
      </button>
    </div>
    <NodeViewContent class="callout-content min-h-[1.5em]" />
  </NodeViewWrapper>
</template>

<style scoped>
.callout-content :deep(p) {
  margin: 0.25em 0;
}
</style>
