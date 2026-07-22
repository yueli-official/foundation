<script setup lang="ts">
import {
  createCollectionRouteQueryCodec,
  createJsonCollectionQueryPolicy,
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
const pageSizes = [10, 20, 40].map((value) => ({
  label: `${value}/页`,
  value,
}));
const categoryLabels = Object.fromEntries(
  categories.map((item) => [item.value, item.label]),
);
const statusLabels: Record<Exclude<Status, "all">, string> = {
  draft: "草稿",
  published: "已发布",
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

const policy = createJsonCollectionQueryPolicy<CollectionQuery>();
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
const { snapshot: collection, workflow } = useVueCollectionWorkflow({
  initialQuery: defaultQuery,
  queryPolicy: policy,
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
const totalPages = computed(() =>
  Math.max(1, Math.ceil(collection.value.total / query.value.size)),
);
const firstVisible = computed(() =>
  collection.value.total === 0
    ? 0
    : (query.value.page - 1) * query.value.size + 1,
);
const lastVisible = computed(() =>
  Math.min(collection.value.total, query.value.page * query.value.size),
);
const activeFilterCount = computed(
  () =>
    [query.value.category, query.value.tag, query.value.status].filter(
      (value) => value !== "all",
    ).length,
);

function updateQuery(patch: Partial<CollectionQuery>, resetPage = true) {
  workflow.setQuery({
    ...query.value,
    ...patch,
    ...(resetPage ? { page: 1 } : {}),
  });
}
function applySearch() {
  updateQuery({ q: searchInput.value.trim() });
}
function clearFilters() {
  updateQuery({ category: "all", tag: "all", status: "all" });
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
                Yueli Foundation UI Lab
              </p>
              <p class="hidden text-xs text-muted sm:block">
                public Pattern laboratory
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
              Experimental public pattern
            </p>
            <h1
              class="text-2xl font-semibold tracking-tight text-highlighted sm:text-3xl"
            >
              内容集合
            </h1>
            <p class="mt-2 max-w-2xl text-sm leading-6 text-muted">
              同一个大框架容纳搜索、筛选、批量选择、数据区与分页；业务数据仍由调用方拥有。
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

        <YCollectionFrame
          v-model:controls-open="filtersOpen"
          labelledby="collection-heading"
          bulk-label="批量操作"
          :bulk-visible="collection.selection.count > 0"
        >
          <h2 id="collection-heading" class="sr-only">内容集合结果</h2>

          <template #search="{ controlsId, controlsOpen, toggleControls }">
            <form
              class="grid grid-cols-[minmax(0,1fr)_auto] gap-2"
              role="search"
              @submit.prevent="applySearch"
            >
              <UInput
                v-model="searchInput"
                icon="i-tabler-search"
                size="sm"
                placeholder="搜索名称、描述或内容 ID"
                class="min-w-0"
              />
              <UButton
                type="submit"
                icon="i-tabler-search"
                label="搜索"
                color="neutral"
                variant="outline"
                size="sm"
              />
            </form>

            <div class="mt-3 flex items-center justify-between gap-2 sm:hidden">
              <UButton
                icon="i-tabler-adjustments-horizontal"
                :label="
                  activeFilterCount ? `筛选 ${activeFilterCount}` : '筛选'
                "
                :aria-controls="controlsId"
                :aria-expanded="controlsOpen"
                color="neutral"
                variant="outline"
                size="xs"
                @click="toggleControls"
              />
              <span class="text-xs text-muted">{{ collection.total }} 项</span>
            </div>
          </template>

          <template #controls>
            <USelect
              :model-value="query.category"
              :items="categories"
              value-key="value"
              size="xs"
              class="w-32"
              aria-label="分类"
              @update:model-value="updateQuery({ category: String($event) })"
            />
            <USelect
              :model-value="query.tag"
              :items="tags"
              value-key="value"
              size="xs"
              class="w-32"
              aria-label="标签"
              @update:model-value="updateQuery({ tag: String($event) })"
            />
            <USelect
              :model-value="query.status"
              :items="statuses"
              value-key="value"
              size="xs"
              class="w-32"
              aria-label="状态"
              @update:model-value="
                updateQuery({ status: String($event) as Status })
              "
            />
            <div class="flex items-center gap-1">
              <USelect
                :model-value="query.sort"
                :items="sorts"
                value-key="value"
                size="xs"
                class="w-32"
                aria-label="排序字段"
                @update:model-value="
                  updateQuery({ sort: String($event) as Sort })
                "
              />
              <UButton
                :icon="
                  query.direction === 'asc'
                    ? 'i-tabler-sort-ascending'
                    : 'i-tabler-sort-descending'
                "
                :aria-label="
                  query.direction === 'asc'
                    ? '当前正序，切换为倒序'
                    : '当前倒序，切换为正序'
                "
                color="neutral"
                variant="outline"
                size="xs"
                square
                @click="
                  updateQuery({
                    direction: query.direction === 'asc' ? 'desc' : 'asc',
                  })
                "
              />
            </div>
            <UButton
              v-if="activeFilterCount"
              label="清除"
              color="neutral"
              variant="ghost"
              size="xs"
              @click="clearFilters"
            />
          </template>

          <template #bulk>
            <div class="flex min-w-0 items-center gap-2 text-xs">
              <span
                class="grid size-6 place-items-center rounded-md bg-primary/10 font-semibold text-primary"
                >{{ collection.selection.count }}</span
              >
              <span class="truncate text-toned"
                >已选择{{
                  collection.selection.mode === "query" ? "全部筛选结果" : "项"
                }}</span
              >
            </div>
            <div class="flex items-center gap-1">
              <UButton
                v-if="
                  collection.selection.mode === 'keys' &&
                  collection.selection.count < collection.total
                "
                label="选择全部结果"
                color="neutral"
                variant="ghost"
                size="xs"
                @click="workflow.selectAllResults()"
              />
              <UButton label="归档" icon="i-tabler-archive" size="xs" />
              <UButton
                label="取消"
                color="neutral"
                variant="ghost"
                size="xs"
                @click="workflow.clearSelection()"
              />
            </div>
          </template>

          <template #columns>
            <div
              class="grid grid-cols-[2.25rem_minmax(0,1fr)_5rem] items-center text-[0.6875rem] font-medium text-muted sm:grid-cols-[2.25rem_minmax(0,1fr)_7rem_8rem_5rem]"
            >
              <UCheckbox
                :model-value="
                  collection.isPageSelected
                    ? true
                    : collection.isPageIndeterminate
                      ? 'indeterminate'
                      : false
                "
                aria-label="选择当前页"
                @update:model-value="workflow.togglePage($event === true)"
              />
              <span>名称</span>
              <span class="hidden sm:block">分类</span>
              <span class="hidden sm:block">更新日期</span>
              <span class="text-right">状态</span>
            </div>
          </template>

          <div v-if="collection.items.length">
            <article
              v-for="item in collection.items"
              :key="item.id"
              class="grid grid-cols-[2.25rem_minmax(0,1fr)_5rem] items-center border-b border-default px-3 py-3 last:border-b-0 sm:grid-cols-[2.25rem_minmax(0,1fr)_7rem_8rem_5rem] sm:px-4"
            >
              <UCheckbox
                :model-value="workflow.isSelected(item.id)"
                :aria-label="`选择 ${item.title}`"
                @update:model-value="workflow.toggleKey(item.id)"
              />
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
            </article>
          </div>
          <div
            v-else
            class="grid min-h-56 place-items-center px-6 py-12 text-center"
          >
            <div>
              <span
                class="mx-auto grid size-10 place-items-center rounded-full bg-muted text-muted"
                ><UIcon name="i-tabler-search-off" class="size-5"
              /></span>
              <p class="mt-3 text-sm font-medium text-highlighted">
                没有匹配结果
              </p>
              <p class="mt-1 text-xs text-muted">
                调整关键词或清除筛选后再试。
              </p>
            </div>
          </div>

          <template #footer>
            <div
              class="flex flex-col gap-3 text-xs sm:flex-row sm:items-center sm:justify-between"
            >
              <p class="text-muted">
                显示 {{ firstVisible }}–{{ lastVisible }}，共
                {{ collection.total }} 项
              </p>
              <div
                class="flex flex-col items-start gap-2 sm:flex-row sm:items-center"
              >
                <UPagination
                  :page="query.page"
                  :total="collection.total"
                  :items-per-page="query.size"
                  :show-edges="false"
                  :sibling-count="1"
                  size="xs"
                  @update:page="updateQuery({ page: $event }, false)"
                />
                <div class="flex items-center gap-2">
                  <span class="text-muted">每页</span>
                  <USelect
                    :model-value="query.size"
                    :items="pageSizes"
                    value-key="value"
                    size="xs"
                    class="w-24"
                    aria-label="每页数量"
                    @update:model-value="updateQuery({ size: Number($event) })"
                  />
                </div>
              </div>
            </div>
          </template>
        </YCollectionFrame>

        <p class="mt-4 text-xs leading-5 text-muted">
          当前为 UI Lab recipe，正式实现来自 `@yueli/ui`，仍未达到
          stable。总页数
          {{ totalPages }}；query、selection 与 load sequencing 来自
          `@yueli/ui/collection`。
        </p>
      </main>
      <YBackToTop label="返回顶部" :threshold="0.5" />
    </div>
  </UApp>
</template>
