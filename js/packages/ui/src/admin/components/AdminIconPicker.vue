<script setup lang="ts">
import { computed } from "vue";
import type { AdminIconOption } from "../types";

const defaults: AdminIconOption[] = [
  { label: "应用", value: "i-tabler-apps" },
  { label: "网站", value: "i-tabler-world-www" },
  { label: "文档", value: "i-tabler-book-2" },
  { label: "代码", value: "i-tabler-code" },
  { label: "工具", value: "i-tabler-tool" },
  { label: "终端", value: "i-tabler-terminal-2" },
  { label: "数据", value: "i-tabler-database" },
  { label: "设置", value: "i-tabler-settings" },
  { label: "发布", value: "i-tabler-rocket" },
  { label: "链接", value: "i-tabler-link" },
  { label: "星标", value: "i-tabler-star" },
  { label: "实验", value: "i-tabler-flask" },
];

const props = withDefaults(defineProps<{
  modelValue?: string;
  options?: AdminIconOption[];
  disabled?: boolean;
}>(), { modelValue: "", options: () => [], disabled: false });
const emit = defineEmits<{ "update:modelValue": [value: string] }>();
const items = computed(() => props.options.length ? props.options : defaults);
const selected = computed(() => props.modelValue || items.value[0]?.value || "i-tabler-apps");

function choose(value: string) {
  if (!props.disabled) emit("update:modelValue", value);
}
</script>

<template>
  <div class="grid grid-cols-[repeat(auto-fill,2.25rem)] gap-1.5" data-admin-icon-picker>
    <UTooltip v-for="item in items" :key="item.value" :text="item.label">
      <UButton
        :icon="item.value"
        :color="selected === item.value ? 'primary' : 'neutral'"
        :variant="selected === item.value ? 'soft' : 'outline'"
        size="sm"
        square
        class="grid size-9 place-items-center p-0"
        :ui="{ leadingIcon: 'mx-auto size-4 shrink-0' }"
        :aria-label="`选择${item.label}`"
        :aria-pressed="selected === item.value"
        :disabled="disabled"
        @click="choose(item.value)"
      />
    </UTooltip>
  </div>
</template>
