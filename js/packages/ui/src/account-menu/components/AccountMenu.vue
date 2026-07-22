<script setup lang="ts">
import type { DropdownMenuItem } from "@nuxt/ui";
import { computed } from "vue";

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
const menuItems = computed(() => {
  const groups: DropdownMenuItem[][] = [
    [
      {
        label: displayName.value,
        description: props.name && props.email ? props.email : undefined,
        type: "label" as const,
      },
    ],
  ];

  if (props.contextActions.length)
    groups.push(props.contextActions.map((action) => ({ ...action })));
  if (props.utilityActions.length)
    groups.push(props.utilityActions.map((action) => ({ ...action })));
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
    groups.push([
      {
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
      },
    ]);
  }
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
</script>

<template>
  <UDropdownMenu
    :items="menuItems"
    :content="{ align: 'center', collisionPadding: 12 }"
    :ui="{
      content:
        triggerMode === 'sidebar'
          ? 'w-(--reka-dropdown-menu-trigger-width)'
          : 'w-56',
    }"
  >
    <UButton
      type="button"
      color="neutral"
      variant="ghost"
      :block="triggerMode === 'sidebar'"
      :square="triggerMode === 'collapsed'"
      :class="[
        'min-h-11 gap-2 px-1.5 data-[state=open]:bg-elevated',
        triggerMode === 'sidebar' && 'w-full justify-start',
        triggerMode === 'collapsed' && 'aspect-square justify-center px-0',
      ]"
      :aria-label="messages.openMenu(displayName)"
      aria-haspopup="menu"
    >
      <UAvatar :src="avatarUrl || undefined" :text="initial" size="xs" />
      <span
        v-if="triggerMode !== 'collapsed'"
        :class="[
          'max-w-36 truncate text-sm',
          triggerMode === 'inline' && 'hidden sm:block',
        ]"
      >
        {{ displayName }}
      </span>
      <UIcon
        v-if="triggerMode === 'sidebar'"
        name="i-tabler-chevrons-up-down"
        class="ms-auto size-3.5 text-dimmed"
      />
      <UIcon
        v-else-if="triggerMode === 'inline'"
        name="i-tabler-chevron-down"
        class="hidden size-3.5 text-dimmed sm:block"
      />
    </UButton>
  </UDropdownMenu>
</template>
