<script setup lang="ts">
import { useSlots } from "vue";

withDefaults(
  defineProps<{
    id: string;
    title: string;
    icon?: string;
    bodyClass?: string;
    mainId?: string;
    resizable?: boolean;
  }>(),
  {
    bodyClass: "",
    mainId: "admin-main",
    resizable: false,
  },
);
const slots = useSlots();
</script>

<template>
  <UDashboardPanel
    :id="id"
    :resizable="resizable"
    class="min-w-0"
    :ui="{ body: 'gap-0 overflow-hidden p-0 sm:p-0' }"
  >
    <template #header>
      <UDashboardNavbar :title="title" :icon="icon">
        <template #leading>
          <slot name="leading">
            <UDashboardSidebarCollapse />
          </slot>
        </template>
        <template v-if="slots.title" #title><slot name="title" /></template>
        <template v-if="slots.trailing" #trailing
          ><slot name="trailing"
        /></template>
        <template v-if="slots.actions" #right><slot name="actions" /></template>
      </UDashboardNavbar>

      <UDashboardToolbar
        v-if="slots.toolbar || slots['toolbar-left'] || slots['toolbar-right']"
      >
        <template v-if="slots['toolbar-left']" #left
          ><slot name="toolbar-left"
        /></template>
        <slot name="toolbar" />
        <template v-if="slots['toolbar-right']" #right
          ><slot name="toolbar-right"
        /></template>
      </UDashboardToolbar>
    </template>

    <template #body>
      <main
        :id="mainId"
        tabindex="-1"
        class="flex-1 overflow-y-auto p-4 outline-none sm:p-6"
        :class="bodyClass"
      >
        <slot />
      </main>
    </template>

    <template v-if="slots.footer" #footer><slot name="footer" /></template>
  </UDashboardPanel>
</template>
