<script setup lang="ts">
import type { AccountMenuMessages } from "@yueli/ui/account-menu/pattern";
import {
  createCollectionRouteQueryCodec,
  createJsonCollectionQueryPolicy,
  type CollectionControl,
  type CollectionPanelMessages,
  type CollectionWorkflow,
} from "@yueli/ui/collection";
import { useVueCollectionWorkflow } from "@yueli/ui/collection/vue";
import { createVueRouterCollectionQuerySync } from "@yueli/ui/collection/vue-router";
import type { DashboardMessages } from "@yueli/ui/dashboard/pattern";
import { useActionFeedback } from "@yueli/ui/feedback";
import { bindSettingsBeforeUnload } from "@yueli/ui/settings/browser";
import type { SettingsSaveDockMessages } from "@yueli/ui/settings/pattern";
import { useVueSettingsWorkflow } from "@yueli/ui/settings/vue";
import { useSettingsLeaveGuard } from "@yueli/ui/settings/vue-router";

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
const settingsFeedback = useActionFeedback({ resetMs: 1_500 });
const settingsForm = reactive({
  title: "公共工作区",
  description: "跨应用设置流程",
});
const settings = useVueSettingsWorkflow({
  snapshot: () => settingsForm,
  restore: (snapshot) => Object.assign(settingsForm, snapshot),
});
const settingsMessages: SettingsSaveDockMessages = {
  region: "设置保存操作",
  unsaved: "有未保存的更改",
  saving: "正在保存更改",
  saved: "更改已保存",
  failed: "保存失败",
  discard: "放弃",
  save: "保存",
  savePending: "保存中",
  saveSuccess: "已保存",
};
const dashboardMessages: DashboardMessages = {
  metrics: "关键指标",
  pending: { title: "待处理", description: "优先处理阻塞工作。" },
  recent: { title: "最近工作", description: "继续最近更新的内容。" },
  health: { title: "运行状态", description: "当前服务状态。" },
  quickActions: { title: "快捷动作", description: "常用的下一步。" },
};
const accountMessages: AccountMenuMessages = {
  currentUser: "当前用户",
  logout: "退出登录",
  openMenu: (name) => `打开 ${name} 的用户菜单`,
};
let unbindBeforeUnload: (() => void) | undefined;
onMounted(() => {
  unbindBeforeUnload = bindSettingsBeforeUnload({
    isDirty: () => settings.dirty.value,
  });
});
onScopeDispose(() => unbindBeforeUnload?.());
useSettingsLeaveGuard({
  isDirty: () => settings.dirty.value,
  confirm: () => window.confirm("有未保存的更改，确定离开当前页面吗？"),
});
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
function saveSettings() {
  void settingsFeedback.run(async () => {
    await new Promise<void>((resolve) => window.setTimeout(resolve, 250));
    settings.capture();
  });
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
          <div class="flex items-center gap-2">
            <YAccountMenu
              name="Lin"
              email="lin@example.test"
              :messages="accountMessages"
              :context-actions="[
                { label: '工作区', icon: 'i-tabler-layout-dashboard' },
              ]"
              :logout="() => undefined"
            />
            <UColorModeButton size="sm" aria-label="切换颜色模式" />
          </div>
        </div>
      </header>

      <main
        id="main-content"
        tabindex="-1"
        class="mx-auto w-full max-w-7xl px-4 py-8 outline-none sm:px-6 lg:px-8 lg:py-12"
      >
        <YDashboardLayout
          class="mb-6"
          title="内容集合"
          description="搜索、筛选、批量选择、状态、数据区与分页均来自公共 Pattern。"
          :messages="dashboardMessages"
        >
          <template #actions>
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
          </template>
          <template #metrics>
            <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
              <div class="rounded-xl border border-default bg-default p-4">
                <p class="text-xl font-semibold tabular-nums text-highlighted">
                  64
                </p>
                <p class="text-xs text-muted">内容</p>
              </div>
              <div class="rounded-xl border border-default bg-default p-4">
                <p class="text-xl font-semibold tabular-nums text-highlighted">
                  48
                </p>
                <p class="text-xs text-muted">已发布</p>
              </div>
            </div>
          </template>
        </YDashboardLayout>

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

        <YSettingsLayout
          class="mt-16"
          title="设置工作流"
          description="Baseline、dirty、discard、leave guard 与保存反馈均穿过公共 Interface。"
          navigation-label="设置分区"
        >
          <YSettingSection
            title="工作区资料"
            description="字段与持久化仍由调用方拥有。"
          >
            <div class="grid gap-4">
              <UFormField label="名称"
                ><UInput
                  v-model="settingsForm.title"
                  data-testid="settings-title"
                  class="w-full"
              /></UFormField>
              <UFormField label="说明"
                ><UTextarea
                  v-model="settingsForm.description"
                  :rows="3"
                  class="w-full"
              /></UFormField>
            </div>
          </YSettingSection>
          <YSettingsSaveDock
            :dirty="settings.dirty.value"
            :status="settingsFeedback.status.value"
            :messages="settingsMessages"
            @discard="settings.discard"
            @save="saveSettings"
          />
        </YSettingsLayout>
      </main>
      <YBackToTop label="返回顶部" :threshold="0.5" />
    </div>
  </UApp>
</template>
