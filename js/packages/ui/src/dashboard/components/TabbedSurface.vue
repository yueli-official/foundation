<script setup lang="ts">
import { computed } from "vue";

interface TabbedSurfaceItem {
  label: string;
  value: string;
  icon?: string;
  badge?: string | number;
  disabled?: boolean;
}

const active = defineModel<string>({ required: true });
const props = defineProps<{
  items: readonly TabbedSurfaceItem[];
  navigationLabel: string;
}>();
const tabItems = computed(() => [...props.items]);
</script>

<template>
  <section
    class="yueli-card min-w-0 overflow-hidden"
    data-manage-tabbed-surface
  >
    <nav
      :aria-label="navigationLabel"
      class="overflow-x-auto overflow-y-hidden border-b border-default px-3 pt-2 [scrollbar-width:none] sm:px-5 sm:pt-3 [&::-webkit-scrollbar]:hidden"
    >
      <UTabs
        v-model="active"
        :items="tabItems"
        :content="false"
        variant="link"
        class="w-max min-w-full"
        :ui="{ list: 'min-w-max', trigger: 'min-w-max' }"
        :aria-label="navigationLabel"
      />
    </nav>
    <slot />
  </section>
</template>
