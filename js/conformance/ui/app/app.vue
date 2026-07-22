<script setup lang="ts">
import {
  createCollectionRouteQueryCodec,
  createJsonCollectionQueryPolicy,
  type CollectionControl,
  type CollectionPanelMessages,
  type CollectionWorkflow,
} from "@yueli/ui/collection";
import { useVueCollectionWorkflow } from "@yueli/ui/collection/vue";
import { createVueRouterCollectionQuerySync } from "@yueli/ui/collection/vue-router";
import { useActionFeedback } from "@yueli/ui/feedback";

type Status = "all" | "draft" | "published";
type Sort = "updated" | "title";
type Direction = "asc" | "desc";

interface CollectionQuery {
  readonly q: string;
  readonly category: string;
  readonly tag: string;
  readonly status: Status;
  readonly sort: Sort;
  readonly direction: Direction;
  readonly page: number;
  readonly size: number;
}

interface ContentItem {
  readonly id: string;
  readonly title: string;
  readonly description: string;
  readonly category: string;
  readonly tags: readonly string[];
  readonly status: Exclude<Status, "all">;
  readonly updated: string;
}

const defaultQuery: CollectionQuery = {
  q: "",
  category: "all",
  tag: "all",
  status: "all",
  sort: "updated",
  direction: "desc",
  page: 1,
  size: 10,
};
const statuses = [
  { label: "全部状态", value: "all" },
  { label: "草稿", value: "draft" },
  { label: "已发布", value: "published" },
];
const categories = [
  { label: "全部分类", value: "all" },
  { label: "指南", value: "guide" },
  { label: "设计", value: "design" },
  { label: "工程", value: "engineering" },
];
const tags = [
  { label: "全部标签", value: "all" },
  { label: "Nuxt", value: "nuxt" },
  { label: "GoFrame", value: "goframe" },
  { label: "A11y", value: "a11y" },
];
const sorts = [
  { label: "最近更新", value: "updated" },
  { label: "标题", value: "title" },
];
const categoryLabels = Object.fromEntries(
  categories.map((item) => [item.value, item.label]),
);
const statusLabels: Record<Exclude<Status, "all">, string> = {
  draft: "草稿",
  published: "已发布",
};
const messages: CollectionPanelMessages = {
  searchPlaceholder: "搜索名称、描述或内容 ID",
  searchAction: "搜索",
  filtersAction: "筛选",
  activeFilters: (count) => `筛选 ${count}`,
  clearFilters: "清除",
  selectPage: "选择当前页",
  selectItem: (label) => `选择 ${label}`,
  bulkRegion: "批量操作",
  selected: (count, mode) =>
    mode === "query"
      ? `已选择全部筛选结果，共 ${count} 项`
      : `已选择 ${count} 项`,
  selectAllResults: "选择全部结果",
  clearSelection: "取消",
  emptyTitle: "没有匹配结果",
  emptyDescription: "调整关键词或清除筛选后再试。",
  errorTitle: "无法加载集合",
  retry: "重试",
  showing: (first, last, total) => `显示 ${first}–${last}，共 ${total} 项`,
  pageSize: "每页",
  pageSizeControl: "每页数量",
  pageSizeOption: (size) => `${size}/页`,
};
const fixture: readonly ContentItem[] = Array.from(
  { length: 64 },
  (_, index) => {
    const category = ["guide", "design", "engineering"][index % 3] ?? "guide";
    const tag = ["nuxt", "goframe", "a11y"][index % 3] ?? "nuxt";
    return {
      id: `content-${String(index + 1).padStart(3, "0")}`,
      title: `公共内容示例 ${String(index + 1).padStart(3, "0")}`,
      description: "用于验证受控查询、跨页选择与响应式集合布局。",
      category,
      tags: [tag],
      status: index % 4 === 0 ? "draft" : "published",
      updated: new Date(Date.UTC(2026, 6, 22 - (index % 20))).toISOString(),
    };
  },
);

const searchInput = ref("");
const filtersOpen = ref(false);
const actionFeedback = useActionFeedback({ resetMs: 1_500 });
const router = useRouter();
const sync = createVueRouterCollectionQuerySync({
  router,
  codec: createCollectionRouteQueryCodec({
    q: { kind: "string", default: "", maxLength: 120 },
    category: {
      kind: "enum",
      values: ["all", "guide", "design", "engineering"] as const,
      default: "all",
    },
    tag: {
      kind: "enum",
      values: ["all", "nuxt", "goframe", "a11y"] as const,
      default: "all",
    },
    status: {
      kind: "enum",
      values: ["all", "draft", "published"] as const,
      default: "all",
    },
    sort: {
      kind: "enum",
      values: ["updated", "title"] as const,
      default: "updated",
    },
    direction: {
      kind: "enum",
      values: ["asc", "desc"] as const,
      default: "desc",
    },
    page: { kind: "positive-integer", default: 1 },
    size: {
      kind: "positive-integer",
      values: [10, 20, 40] as const,
      default: 10,
    },
  }),
});
const {
  snapshot: collection,
  workflow,
  reload,
} = useVueCollectionWorkflow({
  initialQuery: defaultQuery,
  queryPolicy: createJsonCollectionQueryPolicy<CollectionQuery>(),
  keyOf: (item: ContentItem) => item.id,
  querySync: sync,
  load,
});

async function load(
  query: Readonly<CollectionQuery>,
  activeWorkflow: CollectionWorkflow<ContentItem, string, CollectionQuery>,
) {
  const token = activeWorkflow.beginLoad();
  const needle = query.q.toLocaleLowerCase();
  const matched = fixture
    .filter(
      (item) =>
        !needle ||
        `${item.title} ${item.description} ${item.id}`
          .toLocaleLowerCase()
          .includes(needle),
    )
    .filter(
      (item) => query.category === "all" || item.category === query.category,
    )
    .filter((item) => query.tag === "all" || item.tags.includes(query.tag))
    .filter((item) => query.status === "all" || item.status === query.status)
    .toSorted((left, right) => {
      const compared =
        query.sort === "title"
          ? left.title.localeCompare(right.title)
          : left.updated.localeCompare(right.updated);
      return query.direction === "asc" ? compared : -compared;
    });
  const maxPage = Math.max(1, Math.ceil(matched.length / query.size));
  if (query.page > maxPage) {
    activeWorkflow.setQuery({ ...query, page: maxPage });
    return;
  }
  const start = (query.page - 1) * query.size;
  activeWorkflow.resolveLoad(token, {
    items: matched.slice(start, start + query.size),
    total: matched.length,
  });
}

searchInput.value = collection.value.query.q;
watch(
  () => collection.value.query.q,
  (value) => {
    if (searchInput.value !== value) searchInput.value = value;
  },
);

const query = computed(() => collection.value.query);
const activeFilterCount = computed(
  () =>
    [query.value.category, query.value.tag, query.value.status].filter(
      (value) => value !== "all",
    ).length,
);
const controls = computed<readonly CollectionControl[]>(() => [
  {
    kind: "select",
    id: "category",
    label: "分类",
    value: query.value.category,
    options: categories,
  },
  {
    kind: "select",
    id: "tag",
    label: "标签",
    value: query.value.tag,
    options: tags,
  },
  {
    kind: "select",
    id: "status",
    label: "状态",
    value: query.value.status,
    options: statuses,
  },
  {
    kind: "select",
    id: "sort",
    label: "排序字段",
    value: query.value.sort,
    options: sorts,
  },
  {
    kind: "direction",
    id: "direction",
    label: "排序方向",
    value: query.value.direction,
    ascendingLabel: "当前正序，切换为倒序",
    descendingLabel: "当前倒序，切换为正序",
  },
]);

function updateQuery(patch: Partial<CollectionQuery>, resetPage = true) {
  workflow.setQuery({
    ...query.value,
    ...patch,
    ...(resetPage ? { page: 1 } : {}),
  });
}
function applySearch(value: string) {
  searchInput.value = value;
  updateQuery({ q: value });
}
function changeControl(id: string, value: string | number) {
  if (id === "category") updateQuery({ category: String(value) });
  else if (id === "tag") updateQuery({ tag: String(value) });
  else if (id === "status") updateQuery({ status: String(value) as Status });
  else if (id === "sort") updateQuery({ sort: String(value) as Sort });
  else if (id === "direction")
    updateQuery({ direction: String(value) as Direction });
}
function clearFilters() {
  updateQuery({ category: "all", tag: "all", status: "all" });
}
function toggleItem(key: string, selected: boolean) {
  if (workflow.isSelected(key) !== selected) workflow.toggleKey(key);
}
function simulateAction() {
  void actionFeedback.run(
    () => new Promise<void>((resolve) => window.setTimeout(resolve, 250)),
  );
}
</script>

<template>
  <UApp>
    <div class="min-h-dvh bg-muted/30 text-default">
      <header class="border-b border-default bg-default">
        <div
          class="mx-auto flex min-h-14 w-full max-w-7xl items-center justify-between gap-4 px-4 sm:px-6 lg:px-8"
        >
          <div class="flex min-w-0 items-center gap-2.5">
            <span
              class="grid size-8 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary"
            >
              <UIcon name="i-tabler-box-multiple" class="size-4" />
            </span>
            <div class="min-w-0">
              <p class="truncate text-sm font-semibold text-highlighted">
                Yueli UI Conformance
              </p>
              <p class="hidden text-xs text-muted sm:block">
                public Collection Pattern
              </p>
            </div>
          </div>
          <UColorModeButton size="sm" aria-label="切换颜色模式" />
        </div>
      </header>

      <main
        id="main-content"
        tabindex="-1"
        class="mx-auto w-full max-w-7xl px-4 py-8 outline-none sm:px-6 lg:px-8 lg:py-12"
      >
        <div
          class="mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between"
        >
          <div>
            <p
              class="mb-2 text-xs font-medium uppercase tracking-[0.16em] text-primary"
            >
              Experimental workflow
            </p>
            <h1
              class="text-2xl font-semibold tracking-tight text-highlighted sm:text-3xl"
            >
              内容集合
            </h1>
            <p class="mt-2 max-w-2xl text-sm leading-6 text-muted">
              搜索、筛选、批量选择、状态、数据区与分页均来自公共 Pattern。
            </p>
          </div>
          <div class="flex items-center gap-2">
            <YActionFeedbackButton
              :status="actionFeedback.status.value"
              idle-label="保存更改"
              pending-label="保存中"
              success-label="已保存"
              error-label="保存失败"
              size="sm"
              @click="simulateAction"
            />
            <UButton icon="i-tabler-plus" label="新建内容" size="sm" />
          </div>
        </div>

        <YCollectionPanel
          v-model:search="searchInput"
          v-model:filters-open="filtersOpen"
          labelledby="collection-heading"
          :items="collection.items"
          :item-key="(item: ContentItem) => item.id"
          :item-label="(item: ContentItem) => item.title"
          :controls="controls"
          :messages="messages"
          :state="
            collection.loadState === 'loading' ||
            collection.loadState === 'refreshing'
              ? 'loading'
              : collection.loadState === 'error'
                ? 'error'
                : 'ready'
          "
          :error-message="collection.issue?.key"
          :total="collection.total"
          :page="query.page"
          :page-size="query.size"
          :active-filter-count="activeFilterCount"
          selectable
          :selection-count="collection.selection.count"
          :selection-mode="collection.selection.mode"
          :page-selected="collection.isPageSelected"
          :page-indeterminate="collection.isPageIndeterminate"
          :can-select-all-results="
            collection.selection.mode === 'keys' &&
            collection.selection.count < collection.total
          "
          :is-selected="workflow.isSelected"
          @search="applySearch"
          @control-change="changeControl"
          @clear-filters="clearFilters"
          @retry="reload"
          @toggle-page="workflow.togglePage"
          @toggle-item="toggleItem"
          @select-all-results="workflow.selectAllResults"
          @clear-selection="workflow.clearSelection"
          @page-change="updateQuery({ page: $event }, false)"
          @page-size-change="updateQuery({ size: $event })"
        >
          <h2 id="collection-heading" class="sr-only">内容集合结果</h2>
          <template #bulk-actions
            ><UButton label="归档" icon="i-tabler-archive" size="xs"
          /></template>
          <template #columns>
            <div
              class="grid grid-cols-[minmax(0,1fr)_5rem] items-center sm:grid-cols-[minmax(0,1fr)_7rem_8rem_5rem]"
            >
              <span>名称</span><span class="hidden sm:block">分类</span
              ><span class="hidden sm:block">更新日期</span
              ><span class="text-right">状态</span>
            </div>
          </template>
          <template #item="{ item }">
            <div
              class="grid grid-cols-[minmax(0,1fr)_5rem] items-center sm:grid-cols-[minmax(0,1fr)_7rem_8rem_5rem]"
            >
              <div class="min-w-0 pr-3">
                <p class="truncate text-sm font-medium text-highlighted">
                  {{ item.title }}
                </p>
                <p class="mt-0.5 truncate text-xs text-muted">
                  {{ item.description }}
                </p>
                <p class="mt-1 font-mono text-[0.6875rem] text-muted">
                  {{ item.id }}
                </p>
              </div>
              <span class="hidden text-xs text-toned sm:block">{{
                categoryLabels[item.category]
              }}</span>
              <time
                :datetime="item.updated"
                class="hidden text-xs tabular-nums text-muted sm:block"
                >{{ item.updated.slice(0, 10) }}</time
              >
              <div class="text-right">
                <UBadge
                  :label="statusLabels[item.status]"
                  :color="item.status === 'published' ? 'success' : 'neutral'"
                  variant="subtle"
                  size="sm"
                />
              </div>
            </div>
          </template>
        </YCollectionPanel>
      </main>
      <YBackToTop label="返回顶部" :threshold="0.5" />
    </div>
  </UApp>
</template>
