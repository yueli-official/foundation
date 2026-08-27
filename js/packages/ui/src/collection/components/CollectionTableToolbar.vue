<script setup lang="ts">
import { ref, useId, watch } from "vue";

const props = withDefaults(
  defineProps<{
    label: string;
    searchPlaceholder: string;
    searchAction?: string;
    filterLabel: string;
    filterCount?: number;
    selectionCount?: number;
  }>(),
  {
    filterCount: 0,
    selectionCount: 0,
  },
);

const search = defineModel<string>("search", { default: "" });
const filtersOpen = defineModel<boolean>("filtersOpen", { default: false });
const emit = defineEmits<{ search: [value: string] }>();
const filterPanelId = `y-collection-filter-panel-${useId().replaceAll(":", "")}`;
const searchDraft = ref(search.value);
const isComposing = ref(false);

watch(search, (value) => {
  if (!isComposing.value && searchDraft.value !== value) {
    searchDraft.value = value;
  }
});

function updateSearchDraft(value: string) {
  searchDraft.value = value;
  if (!isComposing.value && search.value !== value) {
    search.value = value;
  }
}

function startComposition() {
  isComposing.value = true;
}

function finishComposition(event: CompositionEvent) {
  isComposing.value = false;
  const value =
    (event.target as HTMLInputElement | null)?.value ?? searchDraft.value;
  searchDraft.value = value;
  if (search.value !== value) {
    search.value = value;
  }
}

function submitSearch() {
  if (isComposing.value) return;
  const value = searchDraft.value.trim();
  if (search.value !== value) {
    search.value = value;
  }
  emit("search", value);
}
</script>

<template>
  <section
    data-collection-table-toolbar
    :aria-label="props.label"
    class="@container"
  >
    <div
      v-if="props.selectionCount > 0"
      data-collection-table-selection
      class="flex min-h-24 items-center border-b border-default bg-primary/5 px-3 py-2 sm:px-4 @xl:min-h-16"
    >
      <div class="min-w-0 flex-1">
        <slot name="selection" :count="props.selectionCount" />
      </div>
    </div>

    <div
      v-else
      data-collection-table-default
      class="grid min-h-24 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 border-b border-default p-3 sm:p-4 @xl:min-h-16 @xl:grid-cols-[minmax(14rem,1fr)_auto]"
    >
      <form
        data-collection-table-search
        :class="[
          props.searchAction ? 'grid grid-cols-[minmax(0,1fr)_auto]' : 'block',
          'col-span-2 min-w-0 gap-2 @xl:col-span-1',
        ]"
        role="search"
        @submit.prevent="submitSearch"
      >
        <UInput
          :model-value="searchDraft"
          icon="i-tabler-search"
          size="sm"
          :placeholder="props.searchPlaceholder"
          class="w-full min-w-0"
          @update:model-value="updateSearchDraft"
          @compositionstart="startComposition"
          @compositionend="finishComposition"
        />
        <UButton
          v-if="props.searchAction"
          type="submit"
          icon="i-tabler-search"
          :label="props.searchAction"
          color="neutral"
          variant="outline"
          size="sm"
        />
      </form>

      <div
        v-if="$slots.filters || $slots.utilities"
        data-collection-table-controls
        class="col-span-2 flex min-w-0 items-center justify-between gap-2 @xl:col-span-1 @xl:justify-end"
      >
        <UPopover v-if="$slots.filters" v-model:open="filtersOpen">
          <UButton
            icon="i-tabler-adjustments-horizontal"
            :label="
              props.filterCount
                ? `${props.filterLabel} · ${props.filterCount}`
                : props.filterLabel
            "
            color="neutral"
            variant="outline"
            size="xs"
            :aria-controls="filterPanelId"
            :aria-expanded="filtersOpen"
          />

          <template #content>
            <div
              :id="filterPanelId"
              data-collection-table-filter-panel
              class="p-3"
            >
              <slot name="filters" />
            </div>
          </template>
        </UPopover>

        <div
          v-if="$slots.utilities"
          data-collection-table-utilities
          class="ml-auto flex min-w-0 items-center justify-end gap-1.5"
        >
          <slot name="utilities" />
        </div>
      </div>
    </div>

    <div
      v-if="$slots['active-filters'] && props.filterCount > 0"
      data-collection-table-active-filters
      class="flex min-h-10 flex-wrap items-center gap-1.5 border-b border-default px-3 py-2 sm:px-4"
    >
      <slot name="active-filters" />
    </div>
  </section>
</template>
