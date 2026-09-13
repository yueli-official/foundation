<script setup lang="ts">
import { computed } from "vue";
import CollectionPagination from "./CollectionPagination.vue";
const props = defineProps<{
  page: number;
  pageSize: number;
  total: number;
  pageSizes: readonly number[];
  pageSizeLabel?: string;
  pageSizeControl?: string;
  pageSizeOption?: (value: number) => string;
  label?: string;
}>();
const emit = defineEmits<{
  pageChange: [page: number];
  pageSizeChange: [size: number];
}>();
const options = computed(() =>
  props.pageSizes.map((value) => ({
    value,
    label: props.pageSizeOption?.(value) || `${value} 条 / 页`,
  })),
);
function changeSize(value: unknown) {
  const size = Number(value);
  if (props.pageSizes.includes(size) && size !== props.pageSize)
    emit("pageSizeChange", size);
}
</script>
<template>
  <div
    class="flex min-w-0 flex-wrap items-center justify-between gap-x-3 gap-y-2 text-xs"
    data-collection-pagination-bar
  >
    <div class="flex shrink-0 items-center gap-1" data-pagination-size>
      <USelect
        :model-value="pageSize"
        :items="options"
        value-key="value"
        :aria-label="pageSizeControl || '每页数量'"
        size="xs"
        class="w-[6.5rem]"
        @update:model-value="changeSize"
      />
    </div>
    <CollectionPagination
      :model-value="page"
      :total-pages="Math.ceil(total / pageSize)"
      compact
      :aria-label="label || '分页'"
      class="ml-auto"
      @update:model-value="emit('pageChange', $event)"
    />
  </div>
</template>
<style scoped>
[data-pagination-size] :deep(button) {
  height: 1.75rem;
  min-height: 1.75rem;
}
</style>
