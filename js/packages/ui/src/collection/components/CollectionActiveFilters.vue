<script setup lang="ts">
export interface CollectionActiveFilter {
  key: string;
  label: string;
}
defineProps<{ items: readonly CollectionActiveFilter[] }>();
const emit = defineEmits<{ remove: [key: string]; clear: [] }>();
</script>
<template>
  <div
    v-if="items.length"
    aria-label="已应用筛选"
    class="flex flex-wrap items-center gap-2 text-xs"
  >
    <span class="text-muted">已筛选</span
    ><UButton
      v-for="item in items"
      :key="item.key"
      color="neutral"
      variant="soft"
      size="xs"
      trailing-icon="i-tabler-x"
      :label="item.label"
      @click="emit('remove', item.key)"
    /><UButton
      color="neutral"
      variant="link"
      size="xs"
      label="清除全部"
      @click="emit('clear')"
    />
  </div>
</template>
