<script setup lang="ts">
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

const props = withDefaults(
  defineProps<{
    name?: string;
    email?: string;
    avatarUrl?: string;
    contextActions?: readonly AccountMenuAction[];
    utilityActions?: readonly AccountMenuAction[];
    logout: () => unknown | Promise<unknown>;
    messages: AccountMenuMessages;
  }>(),
  {
    name: "",
    email: "",
    avatarUrl: "",
    contextActions: () => [],
    utilityActions: () => [],
  },
);

const displayName = computed(
  () => props.name || props.email || props.messages.currentUser,
);
const initial = computed(() => displayName.value.charAt(0).toLocaleUpperCase());
const menuItems = computed(() => {
  const groups: Array<Array<AccountMenuAction & { type?: "label" }>> = [
    [
      {
        label: displayName.value,
        description: props.name && props.email ? props.email : undefined,
        type: "label" as const,
      },
    ],
  ];

  if (props.contextActions.length) groups.push([...props.contextActions]);
  if (props.utilityActions.length) groups.push([...props.utilityActions]);
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
  <UDropdownMenu :items="menuItems" :ui="{ content: 'w-56' }">
    <UButton
      type="button"
      color="neutral"
      variant="ghost"
      class="min-h-11 gap-2 px-1.5"
      :aria-label="messages.openMenu(displayName)"
      aria-haspopup="menu"
    >
      <UAvatar :src="avatarUrl || undefined" :text="initial" size="xs" />
      <span class="hidden max-w-36 truncate text-sm sm:block">
        {{ displayName }}
      </span>
      <UIcon
        name="i-tabler-chevron-down"
        class="hidden size-3.5 text-dimmed sm:block"
      />
    </UButton>
  </UDropdownMenu>
</template>
