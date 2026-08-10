<script setup lang="ts">
import { computed, useSlots } from "vue";
import { normalizeAdminNavigation } from "../navigation";
import type {
  AdminNavigationItem,
  AdminSearchGroup,
  AdminShellMessages,
} from "../types";

defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    navigation: readonly AdminNavigationItem[];
    secondaryNavigation?: readonly AdminNavigationItem[];
    searchGroups?: readonly AdminSearchGroup[];
    messages: AdminShellMessages;
    storageKey?: string;
    sidebarId?: string;
    mainId?: string;
    sidebarLabel?: string;
    navigationLabel?: string;
    secondaryNavigationLabel?: string;
    sidebarAppearance?: "framed" | "commercial";
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
    sidebarLabel: "",
    navigationLabel: "",
    secondaryNavigationLabel: "",
    sidebarAppearance: "framed",
    resizable: true,
    collapsible: true,
    defaultSize: 16,
    minSize: 13,
    maxSize: 22,
  },
);
const open = defineModel<boolean>("open", { default: false });
const slots = useSlots();
const normalizedNavigation = computed(() =>
  normalizeAdminNavigation(props.navigation),
);
const navigationStateKey = computed(() =>
  normalizedNavigation.value
    .map((item, index) =>
      item.children?.some((child) => child.active)
        ? String(item.value || index)
        : "",
    )
    .join("|"),
);

function navigationGroup(item: unknown) {
  return item as AdminNavigationItem;
}
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
      role="complementary"
      :aria-label="
        sidebarLabel ||
        navigationLabel ||
        String(navigation[0]?.label || messages.search)
      "
      :resizable="resizable"
      :collapsible="collapsible"
      :default-size="defaultSize"
      :min-size="minSize"
      :max-size="maxSize"
      :toggle="{ size: 'lg', class: 'size-11 shrink-0' }"
      class="admin-shell-sidebar border-e border-default/80 bg-elevated/45"
      :data-admin-sidebar-appearance="sidebarAppearance"
      :ui="{
        header: 'h-auto min-h-(--ui-header-height) gap-2 px-3 py-2.5',
        body: 'gap-3 px-3 py-2',
        footer:
          sidebarAppearance === 'commercial'
            ? 'border-t border-default/70 px-3 pb-3 pt-2.5 lg:border-t'
            : 'px-3 pb-3 pt-2 lg:border-t-0',
        content: 'bg-default',
      }"
    >
      <template v-if="slots.brand" #header="slotProps">
        <div
          data-admin-sidebar-brand
          :class="[
            'min-w-0',
            slotProps.collapsed
              ? 'contents'
              : sidebarAppearance === 'commercial'
                ? 'w-full'
                : 'w-full rounded-xl bg-default/75 p-1 ring ring-default/80',
          ]"
        >
          <slot name="brand" v-bind="slotProps" />
        </div>
      </template>

      <template #default="{ collapsed }">
        <div
          v-if="searchGroups.length"
          class="px-0.5"
          data-admin-sidebar-search
        >
          <UDashboardSearchButton
            v-if="searchGroups.length"
            :collapsed="collapsed"
            :label="messages.search"
            :class="[
              'min-h-11 w-full rounded-lg',
              sidebarAppearance === 'commercial'
                ? 'bg-default/50 ring-default/70 hover:bg-default/80'
                : 'bg-default/75 ring-default hover:bg-default',
            ]"
          />
        </div>

        <slot name="sidebar-top" :collapsed="collapsed" />

        <div class="admin-shell-nav min-w-0" data-admin-sidebar-primary>
          <UNavigationMenu
            :key="navigationStateKey"
            :collapsed="collapsed"
            :items="normalizedNavigation"
            :aria-label="
              navigationLabel ||
              String(normalizedNavigation[0]?.label || messages.search)
            "
            orientation="vertical"
            tooltip
            popover
            :ui="{
              root: 'gap-1',
              list: 'space-y-0.5',
              link: 'min-h-11 gap-2.5 rounded-lg px-2.5 py-2 before:inset-0 before:rounded-lg',
              linkLeadingIcon: 'size-5',
            }"
          >
            <template #admin-navigation-group="{ item }">
              <UIcon
                v-if="navigationGroup(item).icon"
                :name="navigationGroup(item).icon"
                data-slot="linkLeadingIcon"
                data-admin-navigation-group-icon
                class="size-5 shrink-0"
              />
              <span
                data-slot="linkLabel"
                data-admin-navigation-group-label
                class="min-w-0 flex-1 truncate text-start"
              >
                {{ navigationGroup(item).label }}
              </span>
              <UIcon
                name="i-tabler-chevron-down"
                data-slot="linkTrailingIcon"
                data-admin-navigation-group-chevron
                class="size-4 shrink-0 text-dimmed transition-transform duration-200 group-data-[state=open]:rotate-180"
              />
            </template>
          </UNavigationMenu>
        </div>

        <div
          v-if="secondaryNavigation.length || slots['sidebar-bottom']"
          class="mt-auto flex min-w-0 flex-col gap-3 border-t border-default/70 pt-3"
          data-admin-sidebar-support
        >
          <UNavigationMenu
            v-if="secondaryNavigation.length"
            :collapsed="collapsed"
            :items="secondaryNavigation.slice()"
            :aria-label="
              secondaryNavigationLabel ||
              String(secondaryNavigation[0]?.label || messages.search)
            "
            orientation="vertical"
            tooltip
            class="admin-shell-nav"
            :ui="{
              root: 'gap-1',
              list: 'space-y-0.5',
              link: 'min-h-11 gap-2.5 rounded-lg px-2.5 py-2 before:inset-0 before:rounded-lg',
              linkLeadingIcon: 'size-5',
            }"
          />

          <slot name="sidebar-bottom" :collapsed="collapsed" />
        </div>
      </template>

      <template v-if="slots['sidebar-footer']" #footer="slotProps">
        <div
          data-admin-sidebar-account
          :class="[
            'min-w-0',
            slotProps.collapsed
              ? 'contents'
              : sidebarAppearance === 'commercial'
                ? 'w-full'
                : 'w-full rounded-xl bg-default/75 p-1 ring ring-default/80',
          ]"
        >
          <slot name="sidebar-footer" v-bind="slotProps" />
        </div>
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

<style scoped>
.admin-shell-sidebar[data-collapsed="true"] > [data-slot="header"],
.admin-shell-sidebar[data-collapsed="true"] > [data-slot="footer"] {
  padding-inline: 0.5rem;
}

.admin-shell-sidebar[data-collapsed="true"] > [data-slot="body"] {
  gap: 0.5rem;
  padding-inline: 0.5rem;
}

.admin-shell-sidebar[data-collapsed="true"]
  :deep([data-admin-navigation-group-label]),
.admin-shell-sidebar[data-collapsed="true"]
  :deep([data-admin-navigation-group-chevron]) {
  display: none;
}

.admin-shell-nav :deep([data-slot="link"]) {
  border-radius: 0.625rem;
}

.admin-shell-nav :deep([data-slot="link"]::before) {
  border-radius: 0.625rem;
}

.admin-shell-nav :deep([data-slot="link"][data-active]) {
  font-weight: 600;
}

.admin-shell-sidebar[data-admin-sidebar-appearance="framed"]
  .admin-shell-nav
  :deep([data-slot="link"][data-active]) {
  color: var(--ui-primary);
}

.admin-shell-sidebar[data-admin-sidebar-appearance="framed"]
  .admin-shell-nav
  :deep([data-slot="link"][data-active]::before) {
  background: color-mix(in oklab, var(--ui-primary) 9%, var(--ui-bg-default));
  box-shadow: inset 0 0 0 1px
    color-mix(in oklab, var(--ui-primary) 18%, transparent);
}

.admin-shell-sidebar[data-admin-sidebar-appearance="framed"]
  .admin-shell-nav
  :deep([data-slot="link"][data-active]::after) {
  position: absolute;
  inset-block: 0.625rem;
  inset-inline-start: 0.125rem;
  width: 1px;
  border-radius: 9999px;
  background: var(--ui-primary);
  content: "";
}

.admin-shell-sidebar[data-admin-sidebar-appearance="commercial"]
  .admin-shell-nav
  :deep([data-slot="link"][data-active]) {
  color: var(--ui-text-highlighted);
}

.admin-shell-sidebar[data-admin-sidebar-appearance="commercial"]
  .admin-shell-nav
  :deep([data-slot="link"][data-active]::before) {
  background: color-mix(in oklab, var(--ui-primary) 8%, var(--ui-bg-elevated));
  box-shadow: none;
}

.admin-shell-nav
  :deep([data-slot="link"][data-active] [data-slot="linkLeadingIcon"]) {
  color: var(--ui-primary);
}
</style>
