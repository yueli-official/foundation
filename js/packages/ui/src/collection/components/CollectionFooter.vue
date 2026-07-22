<script setup lang="ts">
import { computed } from "vue";
import CollectionDock from "./CollectionDock.vue";
import CollectionPagination from "./CollectionPagination.vue";
const {
  total,
  totalPages,
  label = "选择与分页",
  withSidebar = true,
  pageSizeOptions = [15, 30, 50],
} = defineProps<{
  total: number;
  totalPages: number;
  label?: string;
  withSidebar?: boolean;
  pageSizeOptions?: number[];
}>();
const page = defineModel<number>("page", { required: true });
const size = defineModel<number>("size", { required: true });
const sizeItems = computed(() =>
  pageSizeOptions.map((value) => ({ label: `${value}/页`, value })),
);
</script>
<template>
  <CollectionDock :label="label" :with-sidebar="withSidebar">
    <template #selection
      ><slot name="selection"
        ><span class="text-xs tabular-nums">共 {{ total }} 项</span></slot
      ></template
    >
    <template #pagination
      ><CollectionPagination v-model="page" :total-pages="totalPages"
    /></template>
    <template #page-size
      ><span class="hidden text-xs text-muted xl:inline">每页</span
      ><USelect
        v-model="size"
        :items="sizeItems"
        value-key="value"
        size="xs"
        class="hidden w-24 sm:block"
        aria-label="每页数量"
    /></template>
  </CollectionDock>
</template>
