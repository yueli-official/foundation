<script setup lang="ts">
import { computed, ref } from "vue";
import AdminRowActions from "../../../admin/components/AdminRowActions.vue";
import type { CollectionPanelMessages } from "../../../collection/panel";
import {
  CollectionPanel,
  CollectionLifecycleTabs,
  CollectionSortHeader,
} from "../../../collection/pattern";
import type {
  CommentModerationCollectionActions,
  CommentModerationCollectionModel,
  CommentModerationItem,
} from "../types";

const props = defineProps<{
  model: CommentModerationCollectionModel;
  actions: CommentModerationCollectionActions;
  formatDate: (value: string) => string;
}>();

const search = computed({
  get: () => props.model.search,
  set: (value: string) => props.actions.updateSearch(value),
});
const lifecycle = computed({
  get: () => props.model.lifecycle,
  set: (value) => props.actions.lifecycleChange(value),
});
const lifecycleItems = [
  { key: "all", label: "全部", icon: "i-tabler-messages" },
  { key: "pending", label: "待审核", icon: "i-tabler-inbox" },
  { key: "approved", label: "已通过", icon: "i-tabler-circle-check" },
  { key: "spam", label: "垃圾", icon: "i-tabler-alert-triangle" },
  { key: "trash", label: "回收站", icon: "i-tabler-trash" },
] as const;
const emptyTrashOpen = ref(false);
function openEmptyTrash() {
  emptyTrashOpen.value = true;
}
function closeEmptyTrash() {
  emptyTrashOpen.value = false;
}
async function confirmEmptyTrash() {
  if (!props.actions.emptyTrash) return;
  const result = await props.actions.emptyTrash();
  if (result !== false) emptyTrashOpen.value = false;
}
const messages = computed<CollectionPanelMessages>(() => ({
  searchPlaceholder: props.model.searchPlaceholder,
  searchAction: "搜索",
  filtersAction: "筛选",
  activeFilters: (count) => `筛选（${count}）`,
  clearFilters: "清除筛选",
  selectPage: "选择当前页评论",
  selectItem: (label) => `选择评论：${label}`,
  bulkRegion: "评论批量操作",
  selected: (count) => `已选择 ${count} 条评论`,
  selectAllResults: "选择全部结果",
  clearSelection: "取消选择",
  emptyTitle: "没有匹配的评论",
  emptyDescription: "请调整搜索内容或评论状态后重试。",
  errorTitle: "评论加载失败",
  retry: "重新加载",
  showing: (first, last, total) => `显示 ${first}–${last}，共 ${total} 条`,
  pageSize: "每页",
  pagination: "评论分页",
  pageSizeControl: "每页评论数量",
  pageSizeOption: (value) => `${value} 条`,
}));

const itemKey = (comment: CommentModerationItem) => comment.id;
const itemLabel = (comment: CommentModerationItem) =>
  `${comment.authorName || "匿名用户"}的评论`;
const authorInitial = (name: string) => (name || "?").charAt(0).toUpperCase();
function toggleItem(id: string | number, selected: boolean) {
  props.actions.toggleItem?.(String(id), selected);
}
</script>

<template>
  <div data-comment-moderation-surface>
    <CollectionPanel
    v-model:search="search"
    :items="model.items"
    :item-key="itemKey"
    :item-label="itemLabel"
    :controls="model.controls || []"
    :messages="messages"
    :state="model.state"
    :error-message="model.errorMessage"
    :total="model.total"
    :page="model.page"
    :page-size="model.pageSize"
    :page-sizes="model.pageSizes || [20, 50, 100]"
    :active-filter-count="model.activeFilterCount || 0"
    :selectable="model.selection?.enabled || false"
    :selection-count="model.selection?.count || 0"
    :page-selected="model.selection?.pageSelected || false"
    :page-indeterminate="model.selection?.pageIndeterminate || false"
    :is-selected="model.selection?.isSelected"
    label="评论列表"
    data-manage-comments
    data-comment-moderation-collection
    @search="actions.search"
    @control-change="actions.controlChange"
    @clear-filters="actions.clearFilters"
    @retry="actions.retry"
    @toggle-page="actions.togglePage?.($event)"
    @toggle-item="toggleItem"
    @clear-selection="actions.clearSelection?.()"
    @page-change="actions.pageChange"
    @page-size-change="actions.pageSizeChange"
  >
    <template #navigation>
      <CollectionLifecycleTabs
        v-model="lifecycle"
        :items="lifecycleItems"
        label="评论状态"
      >
        <template #actions>
          <UButton
            v-if="model.lifecycle === 'trash' && actions.emptyTrash"
            label="清空回收站"
            icon="i-tabler-trash-x"
            color="error"
            variant="soft"
            size="xs"
            :disabled="model.total === 0"
            :loading="model.emptyingTrash"
            @click="openEmptyTrash"
          />
        </template>
      </CollectionLifecycleTabs>
    </template>

    <template #columns>
      <div
        class="grid grid-cols-[minmax(0,1fr)_7rem] items-center gap-3 lg:grid-cols-[minmax(16rem,1.4fr)_minmax(10rem,0.8fr)_10rem_8.5rem_7rem]"
      >
        <span>评论</span>
        <span class="hidden lg:block">来源</span>
        <span class="hidden lg:block">用户</span>
        <CollectionSortHeader
          class="hidden lg:inline-flex"
          label="评论日期"
          :active="true"
          :sort-order="model.sortOrder"
          @sort="actions.sort"
        />
        <span class="text-right">操作</span>
      </div>
    </template>

    <template #bulk-actions>
      <slot name="bulk-actions" />
    </template>

    <template #item="{ item: comment }">
      <div
        class="grid min-w-0 grid-cols-[minmax(0,1fr)_7rem] items-start gap-3 lg:grid-cols-[minmax(16rem,1.4fr)_minmax(10rem,0.8fr)_10rem_8.5rem_7rem] lg:items-center"
        data-manage-comment-row
        data-comment-moderation-row
      >
        <div class="min-w-0">
          <p
            class="line-clamp-3 whitespace-pre-wrap text-sm leading-6 text-default"
          >
            {{ comment.content }}
          </p>
          <UBadge
            v-if="comment.status"
            :label="comment.status.label"
            :color="comment.status.color"
            :icon="comment.status.icon"
            variant="subtle"
            size="sm"
            class="mt-1.5 shrink-0"
          />
          <div
            class="mt-1.5 flex min-w-0 flex-wrap items-center gap-1.5 text-xs text-muted lg:hidden"
          >
            <UIcon :name="comment.source.icon" class="size-3.5 shrink-0" />
            <NuxtLink
              v-if="comment.source.to"
              :to="comment.source.to"
              class="max-w-44 truncate hover:text-primary hover:underline"
            >
              {{ comment.source.label }}
            </NuxtLink>
            <span v-else class="max-w-44 truncate">{{ comment.source.label }}</span>
            <span class="text-dimmed">·</span>
            <span class="truncate">{{ comment.authorName }}</span>
            <span v-if="comment.reply" class="text-dimmed">· 回复</span>
            <span class="text-dimmed">·</span>
            <time :datetime="comment.createdAt">
              {{ formatDate(comment.createdAt) }}
            </time>
          </div>
        </div>

        <div class="hidden min-w-0 lg:block">
          <NuxtLink
            v-if="comment.source.to"
            :to="comment.source.to"
            class="flex min-w-0 items-center gap-1.5 text-xs leading-5 text-muted hover:text-primary"
          >
            <UIcon :name="comment.source.icon" class="size-3.5 shrink-0" />
            <span class="line-clamp-2">{{ comment.source.label }}</span>
          </NuxtLink>
          <span v-else class="flex min-w-0 items-center gap-1.5 text-xs text-muted">
            <UIcon :name="comment.source.icon" class="size-3.5 shrink-0" />
            <span class="line-clamp-2">{{ comment.source.label }}</span>
          </span>
        </div>

        <div class="hidden min-w-0 items-center gap-2 lg:flex">
          <UAvatar
            :src="comment.avatarUrl"
            :text="authorInitial(comment.authorName)"
            alt=""
            size="2xs"
            class="shrink-0"
          />
          <div class="min-w-0">
            <div class="flex min-w-0 items-center gap-1.5">
              <span class="truncate text-sm font-medium text-highlighted">
                {{ comment.authorName }}
              </span>
              <UBadge
                v-if="comment.anonymous"
                label="匿名用户"
                color="neutral"
                variant="subtle"
                size="sm"
                class="shrink-0"
              />
            </div>
            <p v-if="comment.authorEmail" class="truncate text-xs text-muted">
              {{ comment.authorEmail }}
            </p>
            <p v-if="comment.reply" class="text-xs text-dimmed">回复</p>
          </div>
        </div>

        <time
          class="hidden text-xs text-muted lg:block"
          :datetime="comment.createdAt"
        >
          {{ formatDate(comment.createdAt) }}
        </time>

        <div class="flex flex-nowrap items-center justify-end gap-1">
          <UButton
            v-if="comment.approve"
            label="通过"
            icon="i-tabler-check"
            size="xs"
            color="success"
            variant="soft"
            :loading="comment.approving"
            @click="actions.approve?.(comment.id)"
          />
          <AdminRowActions
            v-if="comment.actions?.length"
            :items="comment.actions"
            :label="`评论操作：${comment.authorName}`"
            presentation="overflow"
          />
        </div>
      </div>
    </template>
    </CollectionPanel>

    <UModal
      v-model:open="emptyTrashOpen"
      title="清空回收站"
      description="回收站中的评论及其回复将被永久删除，此操作不可撤销。"
      :ui="{ footer: 'justify-end' }"
    >
      <template #footer>
        <UButton
          label="取消"
          color="neutral"
          variant="outline"
          :disabled="model.emptyingTrash"
          @click="closeEmptyTrash"
        />
        <UButton
          label="永久删除全部"
          icon="i-tabler-trash-x"
          color="error"
          :loading="model.emptyingTrash"
          @click="confirmEmptyTrash"
        />
      </template>
    </UModal>
  </div>
</template>
