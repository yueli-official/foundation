<script setup lang="ts">
import type {
  CollectionControl,
  CollectionPanelMessages,
} from "@yueli/ui/collection";
import type {
  AdminNavigationItem,
  AdminSearchGroup,
  AdminShellMessages,
} from "@yueli/ui/admin";
import type {
  RemoteSelectLoader,
  RemoteSelectMessages,
  RemoteSelectValue,
} from "@yueli/ui/remote-select";
import { useActionFeedback } from "@yueli/ui/feedback";

interface LabItem {
  readonly id: string;
  readonly title: string;
  readonly description: string;
  readonly category: "design" | "engineering";
  readonly status: "draft" | "published";
}

const shellMessages: AdminShellMessages = {
  skipToContent: "跳到主要内容",
  search: "搜索",
  searchPlaceholder: "搜索页面与操作",
};
const navigation: readonly AdminNavigationItem[] = [
  { label: "内容集合", icon: "i-tabler-files", to: "/", active: true },
  { label: "设置", icon: "i-tabler-settings", to: "/settings" },
];
const searchGroups: readonly AdminSearchGroup[] = [
  {
    id: "pages",
    label: "页面",
    items: [
      { label: "内容集合", icon: "i-tabler-files", to: "/" },
      { label: "设置", icon: "i-tabler-settings", to: "/settings" },
    ],
  },
];
const ownerMessages: RemoteSelectMessages = {
  placeholder: "选择负责人",
  searchPlaceholder: "搜索负责人",
  empty: "没有匹配的负责人",
  error: "负责人加载失败",
  retry: "重试",
  minimumQuery: (count) => `至少输入 ${count} 个字符`,
};
const selectedOwner = ref<RemoteSelectValue | null>(null);
const loadOwners: RemoteSelectLoader = async ({ query, signal }) => {
  signal.throwIfAborted();
  const needle = query.toLocaleLowerCase();
  return {
    items: [
      { value: "alex", label: "Alex Chen", description: "Content operations" },
      { value: "sam", label: "Sam Lee", description: "Platform engineering" },
      { value: "taylor", label: "Taylor Wu", description: "Design systems" },
    ].filter((item) => item.label.toLocaleLowerCase().includes(needle)),
  };
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
    mode === "query" ? `已选择全部 ${count} 项` : `已选择 ${count} 项`,
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
const allItems: readonly LabItem[] = Array.from({ length: 36 }, (_, index) => ({
  id: `lab-${String(index + 1).padStart(3, "0")}`,
  title: `Foundation Pattern ${String(index + 1).padStart(2, "0")}`,
  description: "UI Lab 只提供场景数据；布局与交互来自公开 CollectionPanel。",
  category: index % 2 === 0 ? "design" : "engineering",
  status: index % 4 === 0 ? "draft" : "published",
}));

const searchInput = ref("");
const appliedSearch = ref("");
const filtersOpen = ref(false);
const category = ref("all");
const status = ref("all");
const sort = ref("title");
const direction = ref<"asc" | "desc">("asc");
const page = ref(1);
const pageSize = ref(10);
const selectedKeys = ref<ReadonlySet<string>>(new Set());
const actionFeedback = useActionFeedback({ resetMs: 1_500 });

const filtered = computed(() => {
  const needle = appliedSearch.value.toLocaleLowerCase();
  const items = allItems
    .filter(
      (item) =>
        !needle ||
        `${item.title} ${item.description} ${item.id}`
          .toLocaleLowerCase()
          .includes(needle),
    )
    .filter(
      (item) => category.value === "all" || item.category === category.value,
    )
    .filter((item) => status.value === "all" || item.status === status.value)
    .toSorted((left, right) => left.title.localeCompare(right.title));
  return direction.value === "asc" ? items : items.toReversed();
});
const items = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return filtered.value.slice(start, start + pageSize.value);
});
const activeFilterCount = computed(
  () =>
    [category.value, status.value].filter((value) => value !== "all").length,
);
const pageSelectionCount = computed(
  () => items.value.filter((item) => selectedKeys.value.has(item.id)).length,
);
const pageSelected = computed(
  () =>
    items.value.length > 0 && pageSelectionCount.value === items.value.length,
);
const pageIndeterminate = computed(
  () => pageSelectionCount.value > 0 && !pageSelected.value,
);
const controls = computed<readonly CollectionControl[]>(() => [
  {
    kind: "select",
    id: "category",
    label: "分类",
    value: category.value,
    options: [
      { label: "全部分类", value: "all" },
      { label: "设计", value: "design" },
      { label: "工程", value: "engineering" },
    ],
  },
  {
    kind: "select",
    id: "status",
    label: "状态",
    value: status.value,
    options: [
      { label: "全部状态", value: "all" },
      { label: "草稿", value: "draft" },
      { label: "已发布", value: "published" },
    ],
  },
  {
    kind: "select",
    id: "sort",
    label: "排序字段",
    value: sort.value,
    options: [{ label: "标题", value: "title" }],
  },
  {
    kind: "direction",
    id: "direction",
    label: "排序方向",
    value: direction.value,
    ascendingLabel: "升序",
    descendingLabel: "降序",
  },
]);

watch([filtered, pageSize], () => {
  page.value = Math.min(
    page.value,
    Math.max(1, Math.ceil(filtered.value.length / pageSize.value)),
  );
});

function changeControl(id: string, value: string | number) {
  if (id === "category") category.value = String(value);
  else if (id === "status") status.value = String(value);
  else if (id === "sort") sort.value = String(value);
  else if (id === "direction")
    direction.value = String(value) as "asc" | "desc";
  page.value = 1;
}
function clearFilters() {
  category.value = "all";
  status.value = "all";
  page.value = 1;
}
function replaceSelection(keys: Iterable<string>) {
  selectedKeys.value = new Set(keys);
}
function toggleItem(key: string, selected: boolean) {
  const next = new Set(selectedKeys.value);
  if (selected) next.add(key);
  else next.delete(key);
  replaceSelection(next);
}
function togglePage(selected: boolean) {
  const next = new Set(selectedKeys.value);
  for (const item of items.value) {
    if (selected) next.add(item.id);
    else next.delete(item.id);
  }
  replaceSelection(next);
}
function simulateAction() {
  void actionFeedback.run(
    () => new Promise<void>((resolve) => window.setTimeout(resolve, 250)),
  );
}
</script>

<template>
  <UApp>
    <YAdminShell
      :navigation="navigation"
      :search-groups="searchGroups"
      :messages="shellMessages"
      storage-key="foundation-ui-lab"
      main-id="admin-main"
    >
      <template #brand="{ collapsed }">
        <div class="flex min-w-0 items-center gap-2.5">
          <span
            class="grid size-8 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary"
          >
            <UIcon name="i-tabler-box-multiple" class="size-4" />
          </span>
          <div v-if="!collapsed" class="min-w-0">
            <p class="truncate text-sm font-semibold text-highlighted">
              Foundation UI
            </p>
            <p class="truncate text-xs text-muted">public pattern lab</p>
          </div>
        </div>
      </template>

      <template #sidebar-footer="{ collapsed }">
        <div class="flex items-center gap-2">
          <UColorModeButton aria-label="切换颜色模式" />
          <span v-if="!collapsed" class="text-xs text-muted">颜色模式</span>
        </div>
      </template>

      <YAdminPage
        id="collection-lab"
        title="内容集合"
        icon="i-tabler-files"
        main-id="admin-main"
        body-class="mx-auto w-full max-w-7xl space-y-6"
      >
        <template #actions>
          <YActionFeedbackButton
            :status="actionFeedback.status.value"
            idle-label="保存更改"
            pending-label="保存中"
            success-label="已保存"
            size="sm"
            @click="simulateAction"
          />
        </template>

        <template #toolbar-left>
          <YRemoteSelect
            v-model="selectedOwner"
            :load="loadOwners"
            :messages="ownerMessages"
            aria-label="负责人"
            class="w-56"
          />
        </template>
        <template #toolbar-right>
          <UBadge label="Experimental" color="warning" variant="subtle" />
        </template>

        <div>
          <p
            class="text-xs font-medium uppercase tracking-[0.16em] text-primary"
          >
            Nuxt UI based public modules
          </p>
          <p class="mt-2 max-w-2xl text-sm leading-6 text-muted">
            Dashboard 壳层、页面 ownership 与远程负责人检索来自公开
            Interface；Lab 只拥有 fixture、领域行和翻译。
          </p>
        </div>

        <YCollectionPanel
          v-model:search="searchInput"
          v-model:filters-open="filtersOpen"
          label="内容集合结果"
          :items="items"
          :item-key="(item: LabItem) => item.id"
          :item-label="(item: LabItem) => item.title"
          :controls="controls"
          :messages="messages"
          :total="filtered.length"
          :page="page"
          :page-size="pageSize"
          :active-filter-count="activeFilterCount"
          selectable
          :selection-count="selectedKeys.size"
          selection-mode="keys"
          :page-selected="pageSelected"
          :page-indeterminate="pageIndeterminate"
          :can-select-all-results="selectedKeys.size < filtered.length"
          :is-selected="(key: string) => selectedKeys.has(key)"
          @search="
            appliedSearch = $event;
            page = 1;
          "
          @control-change="changeControl"
          @clear-filters="clearFilters"
          @toggle-page="togglePage"
          @toggle-item="toggleItem"
          @select-all-results="
            replaceSelection(filtered.map((item) => item.id))
          "
          @clear-selection="replaceSelection([])"
          @page-change="page = $event"
          @page-size-change="
            pageSize = $event;
            page = 1;
          "
        >
          <template #bulk-actions
            ><UButton label="归档" icon="i-tabler-archive" size="xs"
          /></template>
          <template #columns>
            <div class="grid grid-cols-[minmax(0,1fr)_6rem] items-center">
              <span>名称</span><span class="text-right">状态</span>
            </div>
          </template>
          <template #item="{ item }">
            <div class="grid grid-cols-[minmax(0,1fr)_6rem] items-center">
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
              <div class="text-right">
                <UBadge
                  :label="item.status === 'published' ? '已发布' : '草稿'"
                  :color="item.status === 'published' ? 'success' : 'neutral'"
                  variant="subtle"
                  size="sm"
                />
              </div>
            </div>
          </template>
        </YCollectionPanel>
      </YAdminPage>
    </YAdminShell>
  </UApp>
</template>
