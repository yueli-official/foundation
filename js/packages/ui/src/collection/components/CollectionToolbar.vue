<script setup lang="ts">
import { ref } from "vue";
const {
  searchPlaceholder = "搜索…",
  filterCount = 0,
  filterLabel = "筛选",
  compactFilters = false,
} = defineProps<{
  searchPlaceholder?: string;
  filterCount?: number;
  filterLabel?: string;
  compactFilters?: boolean;
}>();
const emit = defineEmits<{ openFilters: [] }>();
const search = defineModel<string>("search", { required: true });
const mobileFiltersOpen = ref(false);
function openFilters() {
  mobileFiltersOpen.value = !mobileFiltersOpen.value;
  emit("openFilters");
}
</script>

<template>
  <section
    aria-label="集合工具栏"
    class="rounded-xl border border-default bg-default p-3 shadow-[var(--shadow-soft)] [container-type:inline-size]"
  >
    <div
      class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3"
      :class="
        compactFilters
          ? '@min-[36rem]:grid-cols-[minmax(12rem,1fr)_auto_auto]'
          : '@min-[80rem]:grid-cols-[minmax(12rem,1fr)_auto_auto]'
      "
    >
      <UInput
        v-model="search"
        icon="i-tabler-search"
        size="sm"
        :placeholder="searchPlaceholder"
        class="col-start-1 row-start-1 min-w-0"
      />
      <div
        v-if="$slots.filters"
        data-collection-controls
        class="col-[1/-1] row-start-3 min-w-0 grid-cols-[minmax(0,1fr)] gap-2 [&>*]:min-w-0 [&>*]:!w-full @min-[36rem]:row-start-2 @min-[36rem]:grid @min-[36rem]:grid-cols-2 @min-[42rem]:grid-cols-3 @min-[52rem]:grid-cols-[repeat(auto-fit,minmax(6.5rem,1fr))]"
        :class="{
          grid: mobileFiltersOpen,
          hidden: !mobileFiltersOpen,
          '@min-[36rem]:col-start-2 @min-[36rem]:row-start-1 @min-[36rem]:grid-cols-[minmax(7.5rem,10rem)_auto] @min-[36rem]:items-center @min-[36rem]:[&>*]:!w-auto':
            compactFilters,
          '@min-[80rem]:col-start-2 @min-[80rem]:row-start-1 @min-[80rem]:!flex @min-[80rem]:flex-nowrap @min-[80rem]:items-center @min-[80rem]:[&>*]:min-w-[7.5rem] @min-[80rem]:[&>*]:max-w-48 @min-[80rem]:[&>*]:flex-[0_1_auto] @min-[80rem]:[&>*]:!w-auto':
            !compactFilters,
        }"
      >
        <slot name="filters" />
      </div>
      <div
        class="col-[1/-1] row-start-2 flex items-center gap-2 @min-[36rem]:hidden"
      >
        <UButton
          v-if="$slots.filters"
          color="neutral"
          variant="outline"
          size="xs"
          icon="i-tabler-adjustments-horizontal"
          :label="filterCount ? `${filterLabel} · ${filterCount}` : filterLabel"
          :aria-expanded="mobileFiltersOpen"
          @click="openFilters"
        />
        <slot name="mobile-actions" />
      </div>
      <div
        v-if="$slots.actions"
        class="col-start-2 row-start-1 flex shrink-0 items-center justify-end gap-2"
        :class="
          compactFilters
            ? '@min-[36rem]:col-start-3'
            : '@min-[80rem]:col-start-3'
        "
      >
        <slot name="actions" />
      </div>
    </div>
  </section>
</template>
