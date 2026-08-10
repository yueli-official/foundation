<script setup lang="ts">
import type { DropdownMenuItem } from "@nuxt/ui";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";

export interface AccountMenuAction {
  readonly label: string;
  readonly icon?: string;
  readonly to?: string;
  readonly description?: string;
  readonly disabled?: boolean;
  readonly onSelect?: (event?: Event) => void | Promise<void>;
}

export interface AccountMenuMessages {
  readonly currentUser: string;
  readonly logout: string;
  readonly openMenu: (displayName: string) => string;
}

export type AccountMenuAppearanceValue = "system" | "light" | "dark";

export interface AccountMenuAppearanceMessages {
  readonly label: string;
  readonly system: string;
  readonly light: string;
  readonly dark: string;
}

export interface AccountMenuAppearance {
  readonly value: AccountMenuAppearanceValue;
  readonly messages: AccountMenuAppearanceMessages;
  readonly onChange: (
    value: AccountMenuAppearanceValue,
  ) => unknown | Promise<unknown>;
}

export type AccountMenuTriggerMode = "inline" | "sidebar" | "collapsed";

const props = withDefaults(
  defineProps<{
    name?: string;
    email?: string;
    avatarUrl?: string;
    contextActions?: readonly AccountMenuAction[];
    utilityActions?: readonly AccountMenuAction[];
    appearance?: AccountMenuAppearance;
    triggerMode?: AccountMenuTriggerMode;
    logout: () => unknown | Promise<unknown>;
    messages: AccountMenuMessages;
  }>(),
  {
    name: "",
    email: "",
    avatarUrl: "",
    contextActions: () => [],
    utilityActions: () => [],
    appearance: undefined,
    triggerMode: "inline",
  },
);

const displayName = computed(
  () => props.name || props.email || props.messages.currentUser,
);
const initial = computed(() => displayName.value.charAt(0).toLocaleUpperCase());
const narrowViewport = ref(false);
let viewportQuery: MediaQueryList | undefined;

function updateViewportMatch(event: MediaQueryList | MediaQueryListEvent) {
  narrowViewport.value = event.matches;
}

onMounted(() => {
  viewportQuery = window.matchMedia("(max-width: 1023px)");
  updateViewportMatch(viewportQuery);
  viewportQuery.addEventListener("change", updateViewportMatch);
});

onBeforeUnmount(() => {
  viewportQuery?.removeEventListener("change", updateViewportMatch);
});

const menuItems = computed(() => {
  const groups: DropdownMenuItem[][] = [
    [
      {
        label: displayName.value,
        description: props.name && props.email ? props.email : undefined,
        avatar: {
          src: props.avatarUrl || undefined,
          text: initial.value,
          alt: displayName.value,
          size: "sm",
        },
        type: "label" as const,
      },
    ],
  ];

  if (props.contextActions.length)
    groups.push(props.contextActions.map((action) => ({ ...action })));

  const utilityItems: DropdownMenuItem[] = props.utilityActions.map(
    (action) => ({ ...action }),
  );
  if (props.appearance) {
    const appearance = props.appearance;
    const options: ReadonlyArray<{
      value: AccountMenuAppearanceValue;
      label: string;
      icon: string;
    }> = [
      {
        value: "system",
        label: appearance.messages.system,
        icon: "i-tabler-device-desktop",
      },
      {
        value: "light",
        label: appearance.messages.light,
        icon: "i-tabler-sun",
      },
      {
        value: "dark",
        label: appearance.messages.dark,
        icon: "i-tabler-moon",
      },
    ];
    utilityItems.push({
      label: appearance.messages.label,
      icon: "i-tabler-sun-moon",
      children: options.map((option) => ({
        label: option.label,
        icon: option.icon,
        type: "checkbox" as const,
        checked: appearance.value === option.value,
        onSelect: async (event?: Event) => {
          event?.preventDefault();
          await appearance.onChange(option.value);
        },
      })),
    });
  }
  if (utilityItems.length) groups.push(utilityItems);

  groups.push([
    {
      label: props.messages.logout,
      icon: "i-tabler-logout",
      onSelect: async () => {
        await props.logout();
      },
    },
  ]);
  return groups;
});

const menuContent = computed(() => ({
  align: "end" as const,
  side:
    props.triggerMode === "inline" || narrowViewport.value
      ? ("bottom" as const)
      : ("right" as const),
  sideOffset: 4,
  collisionPadding: 12,
}));

const menuUi = computed(() => ({
  content: [
    "min-w-56 rounded-xl p-1 shadow-lg",
    props.triggerMode === "sidebar"
      ? "w-(--reka-dropdown-menu-trigger-width)"
      : "w-56",
  ],
  group: "p-0.5",
  label: "gap-2 px-2 py-2 normal-case",
  item: "min-h-9 gap-2.5 rounded-lg px-2 py-1.5",
  itemLeadingIcon: "size-4",
  itemWrapper: "min-w-0",
  itemLabel: "truncate",
  itemDescription: "truncate text-xs",
}));
</script>

<template>
  <UDropdownMenu
    :items="menuItems"
    :content="menuContent"
    :ui="menuUi"
  >
    <UButton
      type="button"
      color="neutral"
      variant="ghost"
      :block="triggerMode === 'sidebar'"
      :square="triggerMode === 'collapsed'"
      :class="[
        'min-h-11 gap-2 px-1.5 data-[state=open]:bg-elevated data-[state=open]:text-highlighted',
        triggerMode === 'sidebar' && 'w-full justify-start',
        triggerMode === 'collapsed' && 'aspect-square justify-center px-0',
      ]"
      :aria-label="messages.openMenu(displayName)"
      aria-haspopup="menu"
      data-account-menu-trigger
    >
      <UAvatar
        :src="avatarUrl || undefined"
        :text="initial"
        :alt="displayName"
        :size="triggerMode === 'sidebar' ? 'sm' : 'xs'"
        class="shrink-0"
      />
      <span
        v-if="triggerMode !== 'collapsed'"
        :class="[
          'grid min-w-0 text-start text-sm leading-tight',
          triggerMode === 'sidebar' ? 'flex-1' : 'max-w-36',
          triggerMode === 'inline' && 'hidden sm:grid',
        ]"
      >
        <span class="truncate font-medium text-highlighted">
          {{ displayName }}
        </span>
        <span
          v-if="triggerMode === 'sidebar' && name && email"
          class="truncate text-xs font-normal text-muted"
        >
          {{ email }}
        </span>
      </span>
      <UIcon
        v-if="triggerMode === 'sidebar'"
        name="i-tabler-selector"
        class="ms-auto size-4 shrink-0 text-muted"
        data-account-menu-indicator
      />
      <UIcon
        v-else-if="triggerMode === 'inline'"
        name="i-tabler-chevron-down"
        class="hidden size-3.5 text-dimmed sm:block"
      />
    </UButton>
  </UDropdownMenu>
</template>
