<script setup lang="ts">
export type CollectionViewMode = "list" | "grid" | "tree" | "table";
export interface CollectionViewOption {
  key: CollectionViewMode;
  label: string;
  icon: string;
}
defineProps<{ items: readonly CollectionViewOption[]; label?: string }>();
const model = defineModel<CollectionViewMode>({ required: true });
function select(key: CollectionViewMode) {
  model.value = key;
}
</script>
<template>
  <div
    role="group"
    :aria-label="label || '展示方式'"
    class="flex items-center rounded-lg border border-default bg-elevated/40 p-0.5"
  >
    <UTooltip v-for="item in items" :key="item.key" :text="item.label"
      ><UButton
        color="neutral"
        :variant="model === item.key ? 'soft' : 'ghost'"
        size="xs"
        :icon="item.icon"
        :aria-label="item.label"
        :aria-pressed="model === item.key"
        square
        @click="select(item.key)"
    /></UTooltip>
  </div>
</template>
