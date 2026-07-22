<script setup lang="ts">
import { computed, useSlots } from "vue";
const { withSidebar = true, label = "集合操作" } = defineProps<{
  withSidebar?: boolean;
  label?: string;
}>();
const hasPageSize = computed(() => Boolean(useSlots()["page-size"]));
</script>
<template>
  <div data-collection-dock-root class="contents">
    <div aria-hidden="true" class="h-32 sm:h-24" />
    <div
      data-collection-dock
      class="pointer-events-none fixed inset-x-0 bottom-0 z-30"
      :class="withSidebar ? 'lg:left-60' : ''"
    >
      <div
        class="mx-auto w-full max-w-screen-2xl px-4 pb-[max(1rem,env(safe-area-inset-bottom))] sm:px-5 lg:px-10"
      >
        <section
          :aria-label="label"
          class="pointer-events-auto min-h-16 gap-3 rounded-xl border border-default bg-default/95 p-3 shadow-[0_-8px_30px_rgba(15,23,42,0.08)] backdrop-blur sm:px-4 dark:shadow-[0_-8px_30px_rgba(0,0,0,0.28)]"
          :class="
            hasPageSize
              ? 'grid grid-cols-[minmax(0,1fr)_auto] items-center sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]'
              : 'flex flex-col sm:flex-row sm:items-center sm:justify-between'
          "
        >
          <div
            class="flex min-w-0 flex-wrap items-center gap-2 text-sm text-muted"
          >
            <slot name="selection" />
          </div>
          <div
            class="flex min-w-0 items-center justify-end gap-2 sm:justify-center"
          >
            <slot name="pagination" />
          </div>
          <div
            v-if="hasPageSize"
            class="hidden min-w-0 items-center justify-end gap-2 sm:flex"
          >
            <slot name="page-size" />
          </div>
        </section>
      </div>
    </div>
  </div>
</template>
