<script
  setup
  lang="ts"
  generic="TItem, TKey extends CollectionKey = CollectionKey"
>
import { computed } from "vue";
import type {
  CollectionControl,
  CollectionControlValue,
  CollectionPanelLayout,
  CollectionPanelMessages,
  CollectionPanelState,
} from "../panel";
import type { CollectionKey } from "../workflow";
import CollectionFrame from "./CollectionFrame.vue";

const props = withDefaults(
  defineProps<{
    label?: string;
    labelledby?: string;
    items: readonly TItem[];
    itemKey: (item: TItem) => TKey;
    itemLabel: (item: TItem) => string;
    controls?: readonly CollectionControl[];
    messages: CollectionPanelMessages;
    state?: CollectionPanelState;
    errorMessage?: string;
    total: number;
    page: number;
    pageSize: number;
    pageSizes?: readonly number[];
    activeFilterCount?: number;
    selectable?: boolean;
    isItemSelectable?: (item: TItem) => boolean;
    selectionCount?: number;
    selectionMode?: "keys" | "query";
    pageSelected?: boolean;
    pageIndeterminate?: boolean;
    canSelectAllResults?: boolean;
    isSelected?: (key: TKey) => boolean;
    layout?: CollectionPanelLayout;
  }>(),
  {
    controls: () => [],
    state: "ready",
    errorMessage: "",
    pageSizes: () => [10, 20, 40],
    activeFilterCount: 0,
    selectable: false,
    selectionCount: 0,
    selectionMode: "keys",
    pageSelected: false,
    pageIndeterminate: false,
    canSelectAllResults: false,
    layout: "rows",
  },
);

const search = defineModel<string>("search", { default: "" });
const filtersOpen = defineModel<boolean>("filtersOpen", { default: false });
const emit = defineEmits<{
  search: [value: string];
  controlChange: [id: string, value: CollectionControlValue];
  clearFilters: [];
  retry: [];
  togglePage: [selected: boolean];
  toggleItem: [key: TKey, selected: boolean];
  selectAllResults: [];
  clearSelection: [];
  pageChange: [page: number];
  pageSizeChange: [pageSize: number];
}>();

const firstVisible = computed(() =>
  props.total === 0 ? 0 : (props.page - 1) * props.pageSize + 1,
);
const lastVisible = computed(() =>
  Math.min(props.total, props.page * props.pageSize),
);
const pageSizeItems = computed(() =>
  props.pageSizes.map((value) => ({
    label: props.messages.pageSizeOption(value),
    value,
  })),
);
const pageSelection = computed(() =>
  props.pageSelected ? true : props.pageIndeterminate ? "indeterminate" : false,
);
const hasControls = computed(() => props.controls.length > 0);

function submitSearch() {
  emit("search", search.value.trim());
}

function changeControl(id: string, value: unknown) {
  if (typeof value === "string" || typeof value === "number") {
    emit("controlChange", id, value);
  }
}

function toggleDirection(
  control: Extract<CollectionControl, { kind: "direction" }>,
) {
  emit("controlChange", control.id, control.value === "asc" ? "desc" : "asc");
}

function selectableItem(item: TItem) {
  return props.isItemSelectable?.(item) ?? true;
}

function selected(item: TItem, key: TKey) {
  return selectableItem(item) && (props.isSelected?.(key) ?? false);
}

function setSelected(item: TItem, key: TKey, next: boolean) {
  if (!selectableItem(item)) return;
  emit("toggleItem", key, next);
}

function toggle(item: TItem, key: TKey) {
  if (!selectableItem(item)) return;
  emit("toggleItem", key, !selected(item, key));
}
</script>

<template>
  <CollectionFrame
    v-model:controls-open="filtersOpen"
    :label="label"
    :labelledby="labelledby"
    :bulk-label="messages.bulkRegion"
    :bulk-visible="selectable && selectionCount > 0"
  >
    <template #search="{ controlsId, controlsOpen, toggleControls }">
      <form
        class="grid grid-cols-[minmax(0,1fr)_auto] gap-2"
        role="search"
        @submit.prevent="submitSearch"
      >
        <UInput
          v-model="search"
          icon="i-tabler-search"
          size="sm"
          :placeholder="messages.searchPlaceholder"
          class="min-w-0"
        />
        <UButton
          type="submit"
          icon="i-tabler-search"
          :label="messages.searchAction"
          color="neutral"
          variant="outline"
          size="sm"
        />
      </form>

      <div
        v-if="hasControls || $slots.view"
        class="mt-3 flex items-center justify-between gap-2 sm:hidden"
      >
        <div class="flex min-w-0 items-center gap-2">
          <UButton
            v-if="hasControls"
            icon="i-tabler-adjustments-horizontal"
            :label="
              activeFilterCount
                ? messages.activeFilters(activeFilterCount)
                : messages.filtersAction
            "
            :aria-controls="controlsId"
            :aria-expanded="controlsOpen"
            color="neutral"
            variant="outline"
            size="xs"
            @click="toggleControls"
          />
          <div v-if="$slots.view" data-collection-mobile-view class="shrink-0">
            <slot name="view" />
          </div>
        </div>
        <span class="text-xs text-muted">{{ total }}</span>
      </div>
    </template>

    <template v-if="hasControls || $slots.view" #controls>
      <template v-for="control in controls" :key="control.id">
        <USelectMenu
          v-if="control.kind === 'select' && control.searchPlaceholder"
          :model-value="control.value"
          :items="control.options.slice()"
          value-key="value"
          :icon="control.icon"
          size="xs"
          :class="control.class ?? 'w-32'"
          :search-input="{ placeholder: control.searchPlaceholder }"
          :aria-label="control.label"
          @update:model-value="changeControl(control.id, $event)"
        />
        <USelect
          v-else-if="control.kind === 'select'"
          :model-value="control.value"
          :items="control.options.slice()"
          value-key="value"
          :icon="control.icon"
          size="xs"
          :class="control.class ?? 'w-32'"
          :aria-label="control.label"
          @update:model-value="changeControl(control.id, $event)"
        />
        <UButton
          v-else
          :icon="
            control.value === 'asc'
              ? 'i-tabler-sort-ascending'
              : 'i-tabler-sort-descending'
          "
          :aria-label="
            control.value === 'asc'
              ? control.ascendingLabel
              : control.descendingLabel
          "
          color="neutral"
          variant="outline"
          size="xs"
          square
          @click="toggleDirection(control)"
        />
      </template>
      <UButton
        v-if="activeFilterCount"
        :label="messages.clearFilters"
        color="neutral"
        variant="ghost"
        size="xs"
        @click="emit('clearFilters')"
      />
      <div
        v-if="$slots.view"
        data-collection-desktop-view
        class="ml-auto hidden sm:block"
      >
        <slot name="view" />
      </div>
    </template>

    <template #bulk>
      <div class="flex min-w-0 items-center gap-2 text-xs">
        <span
          class="grid size-6 place-items-center rounded-md bg-primary/10 font-semibold text-primary"
          >{{ selectionCount }}</span
        >
        <span class="truncate text-toned">{{
          messages.selected(selectionCount, selectionMode)
        }}</span>
      </div>
      <div
        class="flex shrink-0 items-center justify-end gap-1 whitespace-nowrap"
      >
        <UButton
          v-if="canSelectAllResults"
          :label="messages.selectAllResults"
          color="neutral"
          variant="ghost"
          size="xs"
          @click="emit('selectAllResults')"
        />
        <slot name="bulk-actions" />
        <UButton
          :label="messages.clearSelection"
          color="neutral"
          variant="ghost"
          size="xs"
          @click="emit('clearSelection')"
        />
      </div>
    </template>

    <template #columns>
      <div
        class="flex w-full min-w-0 items-center gap-0 text-xs font-medium leading-5 text-muted"
      >
        <div v-if="selectable" class="w-9 shrink-0">
          <UCheckbox
            :model-value="pageSelection"
            :aria-label="messages.selectPage"
            @update:model-value="emit('togglePage', $event === true)"
          />
        </div>
        <div class="min-w-0 flex-1">
          <slot name="columns" />
        </div>
      </div>
    </template>

    <div v-if="state === 'loading'" aria-busy="true" aria-live="polite">
      <slot name="loading">
        <div
          v-for="index in 5"
          :key="index"
          class="flex items-center gap-3 border-b border-default px-3 py-4 last:border-0 sm:px-4"
        >
          <USkeleton v-if="selectable" class="size-4 shrink-0 rounded" />
          <div class="min-w-0 flex-1 space-y-2">
            <USkeleton class="h-4 w-1/3 rounded" />
            <USkeleton class="h-3 w-2/3 rounded" />
          </div>
        </div>
      </slot>
    </div>

    <div
      v-else-if="state === 'error'"
      class="grid min-h-56 place-items-center px-6 py-12 text-center"
      role="alert"
    >
      <slot name="error">
        <div>
          <span
            class="mx-auto grid size-10 place-items-center rounded-full bg-error/10 text-error"
          >
            <UIcon name="i-tabler-alert-circle" class="size-5" />
          </span>
          <p class="mt-3 text-sm font-medium text-highlighted">
            {{ messages.errorTitle }}
          </p>
          <p
            v-if="errorMessage"
            class="mt-1 max-w-md text-xs leading-5 text-muted"
          >
            {{ errorMessage }}
          </p>
          <UButton
            class="mt-4"
            :label="messages.retry"
            color="neutral"
            variant="outline"
            size="xs"
            @click="emit('retry')"
          />
        </div>
      </slot>
    </div>

    <div
      v-else-if="items.length === 0"
      class="grid min-h-56 place-items-center px-6 py-12 text-center"
    >
      <slot name="empty">
        <div>
          <span
            class="mx-auto grid size-10 place-items-center rounded-full bg-muted text-muted"
          >
            <UIcon name="i-tabler-search-off" class="size-5" />
          </span>
          <p class="mt-3 text-sm font-medium text-highlighted">
            {{ messages.emptyTitle }}
          </p>
          <p class="mt-1 text-xs text-muted">{{ messages.emptyDescription }}</p>
        </div>
      </slot>
    </div>

    <div
      v-else-if="layout === 'grid'"
      class="grid gap-3 p-3 sm:grid-cols-2 sm:p-4 lg:grid-cols-3"
    >
      <article
        v-for="item in items"
        :key="itemKey(item)"
        class="relative min-w-0 rounded-lg border bg-default p-4 transition-colors"
        :class="[
          selected(item, itemKey(item))
            ? 'border-primary/50 bg-primary/5 ring-1 ring-primary/20'
            : 'border-default',
        ]"
      >
        <UCheckbox
          v-if="selectable"
          :model-value="selected(item, itemKey(item))"
          :disabled="!selectableItem(item)"
          :aria-label="messages.selectItem(itemLabel(item))"
          class="absolute left-4 top-4 z-10 rounded-md bg-default/90 p-1 shadow-sm backdrop-blur"
          @update:model-value="
            setSelected(item, itemKey(item), $event === true)
          "
        />
        <slot
          name="item"
          :item="item"
          :key="itemKey(item)"
          :selected="selected(item, itemKey(item))"
          :toggle="() => toggle(item, itemKey(item))"
          :selection-active="selectionCount > 0"
        />
      </article>
    </div>

    <div v-else class="divide-y divide-default">
      <article
        v-for="item in items"
        :key="itemKey(item)"
        class="flex min-w-0 items-center px-3 py-3 transition-colors sm:px-4"
        :class="
          selected(item, itemKey(item))
            ? 'bg-elevated/70 hover:bg-elevated/70'
            : 'hover:bg-elevated/40'
        "
      >
        <div v-if="selectable" class="w-9 shrink-0">
          <UCheckbox
            :model-value="selected(item, itemKey(item))"
            :disabled="!selectableItem(item)"
            :aria-label="messages.selectItem(itemLabel(item))"
            @update:model-value="
              setSelected(item, itemKey(item), $event === true)
            "
          />
        </div>
        <div class="min-w-0 flex-1">
          <slot
            name="item"
            :item="item"
            :key="itemKey(item)"
            :selected="selected(item, itemKey(item))"
            :toggle="() => toggle(item, itemKey(item))"
            :selection-active="selectionCount > 0"
          />
        </div>
      </article>
    </div>

    <template #footer>
      <div
        class="flex flex-col gap-3 text-xs sm:flex-row sm:items-center sm:justify-between"
      >
        <p class="text-muted">
          {{ messages.showing(firstVisible, lastVisible, total) }}
        </p>
        <div
          class="flex flex-col items-start gap-2 sm:flex-row sm:items-center"
        >
          <UPagination
            :page="page"
            :total="total"
            :items-per-page="pageSize"
            :aria-label="messages.pagination || messages.pageSize"
            :show-edges="false"
            :sibling-count="1"
            size="xs"
            @update:page="emit('pageChange', $event)"
          />
          <div class="flex items-center gap-2">
            <span class="text-muted">{{ messages.pageSize }}</span>
            <USelect
              :model-value="pageSize"
              :items="pageSizeItems"
              value-key="value"
              size="xs"
              class="w-24"
              :aria-label="messages.pageSizeControl"
              @update:model-value="emit('pageSizeChange', Number($event))"
            />
          </div>
        </div>
      </div>
    </template>
  </CollectionFrame>
</template>
