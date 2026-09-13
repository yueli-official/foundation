<script setup lang="ts">
const props = defineProps<{ totalPages: number; compact?: boolean }>();
const page = defineModel<number>({ required: true });
function go(value: number) {
  if (value >= 1 && value <= props.totalPages) page.value = value;
}
</script>

<template>
  <UPagination
    v-if="compact || totalPages > 1"
    :page="page"
    :total="Math.max(1, totalPages)"
    :items-per-page="1"
    :show-edges="compact"
    :ui="
      compact
        ? { first: '!inline-flex', last: '!inline-flex', list: 'flex-wrap' }
        : undefined
    "
    :data-compact-page-buttons="compact || undefined"
    :sibling-count="1"
    size="xs"
    @update:page="go"
  >
    <template #first
      ><UButton
        aria-label="第一页"
        title="第一页"
        icon="i-tabler-chevrons-left"
        color="neutral"
        variant="outline"
        size="xs"
    /></template>
    <template #prev
      ><UButton
        aria-label="上一页"
        title="上一页"
        icon="i-tabler-chevron-left"
        color="neutral"
        variant="outline"
        size="xs"
    /></template>
    <template #next
      ><UButton
        aria-label="下一页"
        title="下一页"
        icon="i-tabler-chevron-right"
        color="neutral"
        variant="outline"
        size="xs"
    /></template>
    <template #last
      ><UButton
        aria-label="最后一页"
        title="最后一页"
        icon="i-tabler-chevrons-right"
        color="neutral"
        variant="outline"
        size="xs"
    /></template>
    <template #item="{ item, page: activePage }"
      ><UButton
        v-if="item.type === 'page'"
        :aria-label="`第 ${item.value} 页`"
        :label="String(item.value)"
        :color="activePage === item.value ? 'primary' : 'neutral'"
        :variant="activePage === item.value ? 'solid' : 'outline'"
        size="xs"
        square
    /></template>
  </UPagination>
</template>

<style scoped>
[data-compact-page-buttons] :deep(button) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.75rem;
  height: 1.75rem;
  min-width: 1.75rem;
  min-height: 1.75rem;
  padding: 0;
}
</style>
