<script setup lang="ts">
export type CollectionViewMode = "list" | "grid" | "tree" | "table";
export interface CollectionViewOption {
  key: CollectionViewMode;
  label: string;
  icon: string;
}
withDefaults(defineProps<{ items: readonly CollectionViewOption[]; label?: string; appearance?: "default" | "surface" }>(), { appearance: "surface" });
const model = defineModel<CollectionViewMode>({ required: true });
function select(key: CollectionViewMode) {
  model.value = key;
}
</script>
<template>
  <div
    role="group"
    :aria-label="label || '展示方式'"
    class="flex items-center rounded-lg border border-default p-0.5"
    :class="appearance === 'surface' ? 'bg-default' : 'bg-elevated/40'"
    :data-view-appearance="appearance"
  >
    <UTooltip v-for="item in items" :key="item.key" :text="item.label"
      ><UButton
        :color="appearance === 'surface' && model === item.key ? 'primary' : 'neutral'"
        :variant="model === item.key ? 'soft' : 'ghost'"
        size="xs"
        :icon="item.icon"
        :aria-label="item.label"
        :aria-pressed="model === item.key"
        data-collection-view-option
        square
        @click="select(item.key)"
    /></UTooltip>
  </div>
</template>

<style scoped>
[data-view-appearance="surface"] :deep(button) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  min-height: 2rem;
  padding: 0;
}
[data-view-appearance="surface"] :deep(button[aria-pressed="true"]) {
  color: var(--ui-primary);
  background-color: color-mix(in srgb, var(--ui-primary) 12%, var(--ui-bg));
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--ui-primary) 22%, transparent);
}
</style>
