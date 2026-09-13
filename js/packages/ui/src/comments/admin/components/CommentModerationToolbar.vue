<script setup lang="ts">
import { computed } from "vue";
import CollectionTableToolbar from "../../../collection/components/CollectionTableToolbar.vue";
import type {
  CommentModerationCollectionActions,
  CommentModerationCollectionModel,
  CommentModerationLifecycle,
} from "../types";

const props = defineProps<{
  model: CommentModerationCollectionModel;
  actions: CommentModerationCollectionActions;
}>();
const search = computed({
  get: () => props.model.search,
  set: (value) => props.actions.updateSearch(value),
});
const statuses: { label: string; value: CommentModerationLifecycle }[] = [
  { label: "全部评论", value: "all" },
  { label: "待审核", value: "pending" },
  { label: "已通过", value: "approved" },
  { label: "垃圾评论", value: "spam" },
  { label: "回收站", value: "trash" },
];
</script>

<template>
  <CollectionTableToolbar
    v-model:search="search"
    presentation="header"
    label="评论搜索与筛选"
    :search-placeholder="model.searchPlaceholder"
    filter-label="筛选"
    class="ml-auto w-full max-w-full sm:w-[34rem]"
    @search="actions.search"
  >
    <template #utilities>
      <USelect
        :model-value="model.lifecycle"
        :items="statuses"
        value-key="value"
        aria-label="评论状态"
        icon="i-tabler-filter"
        size="sm"
        class="w-32"
        @update:model-value="actions.lifecycleChange($event)"
      />
      <UButton
        :label="model.sortOrder === 'desc' ? '最新优先' : '最早优先'"
        :icon="
          model.sortOrder === 'desc'
            ? 'i-tabler-sort-descending'
            : 'i-tabler-sort-ascending'
        "
        color="neutral"
        variant="outline"
        size="sm"
        aria-label="切换评论时间排序"
        @click="actions.sort"
      />
    </template>
  </CollectionTableToolbar>
</template>

<style scoped>
:deep(button) {
  height: 2.25rem;
  min-height: 2.25rem;
}
</style>
