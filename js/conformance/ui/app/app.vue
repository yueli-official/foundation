<script setup lang="ts">
import type {
  AccountMenuAppearance,
  AccountMenuAppearanceValue,
  AccountMenuMessages,
} from "@yueli/ui/account-menu/pattern";
import type {
  AdminNavigationItem,
  AdminSearchGroup,
  AdminShellMessages,
} from "@yueli/ui/admin";
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
import type {
  RemoteSelectLoader,
  RemoteSelectMessages,
  RemoteSelectOption,
  RemoteSelectValue,
} from "@yueli/ui/remote-select";
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
const adminMessages: AdminShellMessages = {
  skipToContent: "跳到主要内容",
  search: "搜索",
  searchPlaceholder: "搜索页面与操作",
};
const navigation: readonly AdminNavigationItem[] = [
  {
    label: "内容集合",
    icon: "i-tabler-files",
    active: true,
  },
  {
    label: "设置工作流",
    icon: "i-tabler-settings",
  },
];
const secondaryNavigation: readonly AdminNavigationItem[] = [
  {
    label: "使用文档",
    icon: "i-tabler-book-2",
    to: "https://github.com/yueli-official/foundation",
    target: "_blank",
  },
];
const searchGroups: readonly AdminSearchGroup[] = [
  {
    id: "pages",
    label: "页面",
    items: [
      { id: "content", label: "内容集合", icon: "i-tabler-files" },
      { id: "settings", label: "设置工作流", icon: "i-tabler-settings" },
    ],
  },
];
const owners: readonly RemoteSelectOption<string>[] = [
  {
    value: "owner-lin",
    label: "Lin",
    description: "内容负责人",
    icon: "i-tabler-user",
  },
  {
    value: "owner-yue",
    label: "Yue",
    description: "设计系统维护者",
    icon: "i-tabler-user",
  },
  {
    value: "owner-api",
    label: "API Team",
    description: "GoFrame 服务团队",
    icon: "i-tabler-users",
  },
];
const ownerId = ref<RemoteSelectValue | null>("owner-lin");
const ownerMessages: RemoteSelectMessages = {
  placeholder: "选择负责人",
  searchPlaceholder: "搜索负责人",
  empty: "没有匹配的负责人",
  error: "负责人加载失败",
  retry: "重试",
  minimumQuery: (count) => `至少输入 ${count} 个字符`,
};
const loadOwners: RemoteSelectLoader<string> = async ({ query, signal }) => {
  await Promise.resolve();
  if (signal.aborted) return { items: [] };
  const needle = query.toLocaleLowerCase();
  return {
    items: owners.filter((owner) =>
      `${owner.label} ${owner.description ?? ""}`
        .toLocaleLowerCase()
        .includes(needle),
    ),
  };
};
const accountMessages: AccountMenuMessages = {
  currentUser: "当前用户",
  logout: "退出登录",
  openMenu: (name) => `打开 ${name} 的用户菜单`,
};
const colorMode = useColorMode();
const accountAppearance = computed<AccountMenuAppearance>(() => ({
  value: (["light", "dark"] as const).includes(
    colorMode.preference as "light" | "dark",
  )
    ? (colorMode.preference as AccountMenuAppearanceValue)
    : "system",
  messages: {
    label: "外观",
    system: "跟随系统",
    light: "浅色",
    dark: "深色",
  },
  onChange: (value) => {
    colorMode.preference = value;
  },
}));
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
    <YAdminShell
      storage-key="ui-conformance-admin"
      main-id="main-content"
      :navigation="navigation"
      :secondary-navigation="secondaryNavigation"
      :search-groups="searchGroups"
      :messages="adminMessages"
    >
      <template #brand>
        <div class="flex min-w-0 items-center gap-2.5 px-2">
          <span
            class="grid size-8 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary"
          >
            <UIcon name="i-tabler-box-multiple" class="size-4" />
          </span>
          <div class="min-w-0">
            <p class="truncate text-sm font-semibold text-highlighted">
              Yueli UI
            </p>
            <p class="truncate text-xs text-muted">Admin conformance</p>
          </div>
        </div>
      </template>
      <template #sidebar-footer="{ collapsed }">
        <YAccountMenu
          name="Lin"
          email="lin@example.test"
          :messages="accountMessages"
          :context-actions="[
            { label: '工作区', icon: 'i-tabler-layout-dashboard' },
          ]"
          :appearance="accountAppearance"
          :trigger-mode="collapsed ? 'collapsed' : 'sidebar'"
          :logout="() => undefined"
        />
      </template>

      <YAdminPage
        id="content"
        title="内容集合"
        icon="i-tabler-files"
        main-id="main-content"
        body-class="space-y-16 lg:p-8"
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
        <template #toolbar-left>
          <YRemoteSelect
            v-model="ownerId"
            class="w-56"
            aria-label="按负责人筛选"
            :load="loadOwners"
            :initial-items="owners.slice(0, 1)"
            :messages="ownerMessages"
          />
        </template>
        <template #toolbar-right>
          <span class="text-xs text-muted">远程选项 · latest wins</span>
        </template>

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
      </YAdminPage>
      <YBackToTop
        label="返回顶部"
        target-id="main-content"
        scroll-container-id="main-content"
        :threshold="0.5"
      />
    </YAdminShell>
  </UApp>
</template>
