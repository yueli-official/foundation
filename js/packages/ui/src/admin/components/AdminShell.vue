<script setup lang="ts">
import { useSlots } from "vue";
import type {
  AdminNavigationItem,
  AdminSearchGroup,
  AdminShellMessages,
} from "../types";

defineOptions({ inheritAttrs: false });

withDefaults(
  defineProps<{
    navigation: readonly AdminNavigationItem[];
    secondaryNavigation?: readonly AdminNavigationItem[];
    searchGroups?: readonly AdminSearchGroup[];
    messages: AdminShellMessages;
    storageKey?: string;
    sidebarId?: string;
    mainId?: string;
    resizable?: boolean;
    collapsible?: boolean;
    defaultSize?: number;
    minSize?: number;
    maxSize?: number;
  }>(),
  {
    secondaryNavigation: () => [],
    searchGroups: () => [],
    storageKey: "yueli-admin",
    sidebarId: "primary",
    mainId: "admin-main",
    resizable: true,
    collapsible: true,
    defaultSize: 16,
    minSize: 13,
    maxSize: 22,
  },
);
const open = defineModel<boolean>("open", { default: false });
const slots = useSlots();
</script>

<template>
  <UDashboardGroup
    v-bind="$attrs"
    :storage-key="storageKey"
    unit="rem"
    data-admin-shell
  >
    <a
      :href="`#${mainId}`"
      class="sr-only fixed start-3 top-3 z-[100] rounded-md bg-default px-3 py-2 text-sm font-medium text-highlighted shadow focus:not-sr-only"
    >
      {{ messages.skipToContent }}
    </a>

    <UDashboardSidebar
      :id="sidebarId"
      v-model:open="open"
      :resizable="resizable"
      :collapsible="collapsible"
      :default-size="defaultSize"
      :min-size="minSize"
      :max-size="maxSize"
      class="bg-elevated/25"
      :ui="{ footer: 'lg:border-t lg:border-default' }"
    >
      <template v-if="slots.brand" #header="slotProps">
        <slot name="brand" v-bind="slotProps" />
      </template>

      <template #default="{ collapsed }">
        <UDashboardSearchButton
          v-if="searchGroups.length"
          :collapsed="collapsed"
          :label="messages.search"
          class="bg-transparent ring-default"
        />

        <slot name="sidebar-top" :collapsed="collapsed" />

        <UNavigationMenu
          :collapsed="collapsed"
          :items="navigation.slice()"
          orientation="vertical"
          tooltip
          popover
        />

        <UNavigationMenu
          v-if="secondaryNavigation.length"
          :collapsed="collapsed"
          :items="secondaryNavigation.slice()"
          orientation="vertical"
          tooltip
          class="mt-auto"
        />

        <slot name="sidebar-bottom" :collapsed="collapsed" />
      </template>

      <template v-if="slots['sidebar-footer']" #footer="slotProps">
        <slot name="sidebar-footer" v-bind="slotProps" />
      </template>
    </UDashboardSidebar>

    <UDashboardSearch
      v-if="searchGroups.length"
      :groups="searchGroups.slice()"
      :placeholder="messages.searchPlaceholder"
    />

    <slot :main-id="mainId" />
  </UDashboardGroup>
</template>
