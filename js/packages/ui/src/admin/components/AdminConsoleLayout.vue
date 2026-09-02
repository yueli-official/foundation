<script setup lang="ts">
import { computed, ref } from "vue";
import BackToTop from "../../navigation/components/BackToTop.vue";
import AdminShell from "./AdminShell.vue";
import type {
  AdminNavigationItem,
  AdminSearchGroup,
  AdminShellMessages,
  AdminShellUi,
} from "../types";

defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    brandLabel: string;
    brandIcon: string;
    brandTo: string;
    navigation: readonly AdminNavigationItem[];
    secondaryNavigation?: readonly AdminNavigationItem[];
    searchGroups?: readonly AdminSearchGroup[];
    messages: AdminShellMessages;
    storageKey: string;
    mainId: string;
    currentLabel?: string;
    contextLabel?: string;
    immersive?: boolean;
    backToTopLabel?: string;
  }>(),
  {
    secondaryNavigation: () => [],
    searchGroups: () => [],
    currentLabel: "",
    contextLabel: "",
    immersive: false,
    backToTopLabel: "Back to top",
  },
);

const sidebarOpen = ref(false);
const activeItem = computed(
  () => props.navigation.find((item) => item.active) || props.navigation[0],
);
const resolvedCurrentLabel = computed(
  () => props.currentLabel || String(activeItem.value?.label || ""),
);
const resolvedContextLabel = computed(
  () => props.contextLabel || props.brandLabel,
);

function closeSidebar() {
  sidebarOpen.value = false;
}

function navigationWithClose(
  items: readonly AdminNavigationItem[],
): AdminNavigationItem[] {
  return items.map((item) => {
    const original = item.onSelect;
    return {
      ...item,
      children: item.children?.map((child) => {
        const childOriginal = child.onSelect;
        return {
          ...child,
          onSelect: (event: Event) => {
            closeSidebar();
            if (typeof childOriginal === "function") childOriginal(event);
          },
        };
      }),
      onSelect: (event: Event) => {
        closeSidebar();
        if (typeof original === "function") original(event);
      },
    };
  });
}

const shellNavigation = computed(() => navigationWithClose(props.navigation));
const shellSecondaryNavigation = computed(() =>
  navigationWithClose(props.secondaryNavigation),
);

const shellUi: AdminShellUi = {
  sidebar:
    "yueli-admin-shell-surface h-svh min-h-0 max-h-svh overflow-hidden border-e border-default/70",
  sidebarHeader: "shrink-0",
  sidebarBody: "min-h-0 flex-1 overflow-y-auto",
  sidebarFooter: "shrink-0",
  navigationLink:
    "group relative min-h-12 rounded-xl border border-transparent px-2.5 py-2 text-muted transition-colors after:pointer-events-none after:absolute after:start-2.5 after:top-1/2 after:size-8 after:-translate-y-1/2 after:rounded-lg after:bg-muted after:content-[''] hover:border-default hover:bg-primary/5 hover:text-default data-[active]:border-primary/25 data-[active]:bg-primary/10 data-[active]:text-highlighted data-[active]:shadow-[inset_2px_0_var(--ui-primary)] data-[active]:after:bg-primary/10",
  navigationIcon:
    "relative z-10 size-8 bg-current text-muted opacity-100 [mask-position:center] [mask-repeat:no-repeat] [mask-size:1rem_1rem] group-data-[active]:!text-primary",
};
</script>

<template>
  <AdminShell
    v-bind="$attrs"
    v-model:open="sidebarOpen"
    :navigation="shellNavigation"
    :secondary-navigation="shellSecondaryNavigation"
    :messages="messages"
    sidebar-appearance="commercial"
    :storage-key="storageKey"
    :main-id="mainId"
    class="yueli-admin-console relative min-h-svh overflow-clip"
    data-admin-console
    :ui="shellUi"
    :resizable="false"
    :collapsible="false"
    :default-size="16.75"
    :min-size="16.75"
    :max-size="16.75"
  >
    <template #brand="{ collapsed }">
      <NuxtLink
        :to="brandTo"
        :aria-label="collapsed ? brandLabel : undefined"
        class="flex min-h-11 min-w-0 items-center gap-3 text-highlighted"
        @click="closeSidebar"
      >
        <span
          class="grid size-10 shrink-0 place-items-center rounded-xl bg-primary/10 text-primary ring-1 ring-primary/25 shadow-sm"
          data-admin-console-brand-icon
        >
          <UIcon :name="brandIcon" class="size-5" />
        </span>
        <span
          v-if="!collapsed"
          class="min-w-0 truncate text-base font-bold tracking-[0.01em]"
        >
          {{ brandLabel }}
        </span>
      </NuxtLink>
    </template>

    <template #sidebar-footer="{ collapsed }">
      <slot name="account" :collapsed="collapsed" />
    </template>

    <UDashboardPanel
      data-admin-console-panel
      class="h-svh min-h-0 overflow-hidden lg:not-last:border-e-0"
      :ui="{
        body: 'min-h-0 flex-1 gap-0 overflow-hidden p-0 sm:gap-0 sm:p-0',
      }"
    >
      <template #header>
        <UDashboardNavbar
          v-if="!immersive"
          :toggle="{ class: 'size-11 shrink-0 lg:hidden' }"
          class="yueli-admin-shell-surface relative z-20 border-default"
          :ui="{
            root: 'min-h-16 border-b px-4 lg:px-8',
            left: 'min-w-0 gap-3',
            right: 'shrink-0',
          }"
        >
          <template #left>
            <div
              class="flex min-w-0 items-center gap-2 text-xs text-dimmed"
              :aria-label="messages.currentLocation || 'Current location'"
              data-admin-console-breadcrumb
            >
              <span class="hidden sm:inline">{{ resolvedContextLabel }}</span>
              <UIcon
                name="i-tabler-chevron-right"
                class="hidden size-3.5 sm:block"
              />
              <strong class="truncate font-semibold text-toned">
                {{ resolvedCurrentLabel }}
              </strong>
            </div>
          </template>
          <template v-if="$slots['topbar-right']" #right>
            <slot name="topbar-right" />
          </template>
        </UDashboardNavbar>
      </template>

      <template #body>
        <main
          :id="mainId"
          tabindex="-1"
          class="min-h-0 min-w-0 flex-1 overflow-y-auto outline-none"
          :class="
            immersive
              ? 'w-full bg-default'
              : 'yueli-admin-canvas w-full px-4 py-6 sm:px-6 sm:py-8 lg:px-8 lg:pb-14'
          "
          data-admin-console-canvas
        >
          <slot />
        </main>
      </template>
    </UDashboardPanel>

    <BackToTop
      :target-id="mainId"
      :scroll-container-id="mainId"
      avoid-selector="[data-manage-dock], [data-back-to-top-avoid]"
      :label="backToTopLabel"
    />
  </AdminShell>
</template>
