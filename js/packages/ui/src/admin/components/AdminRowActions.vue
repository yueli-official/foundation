<script setup lang="ts">
import { computed } from "vue";
import type { AdminRowActionItem } from "../types";

const props = withDefaults(
  defineProps<{
    items: readonly AdminRowActionItem[] | readonly (readonly AdminRowActionItem[])[];
    label: string;
    presentation?: "inline" | "overflow";
  }>(),
  { presentation: "inline" },
);

function isGroup(value: AdminRowActionItem | readonly AdminRowActionItem[]): value is readonly AdminRowActionItem[] {
  return Array.isArray(value);
}

const groups = computed<readonly (readonly AdminRowActionItem[])[]>(() => {
  const source = props.items ?? [];
  if (!source.length) return [];
  const normalized = isGroup(source[0] as AdminRowActionItem | readonly AdminRowActionItem[])
    ? source as readonly (readonly AdminRowActionItem[])[]
    : [source as readonly AdminRowActionItem[]];
  return normalized
    .map((group) => group.filter((item) => !item.hidden))
    .filter((group) => group.length > 0);
});

const inlineItems = computed(() => groups.value.flat());
const menuItems = computed(() => groups.value.map((group) => group.map((item) => ({
  label: item.label,
  icon: item.icon,
  to: item.to,
  target: item.target,
  rel: item.rel,
  disabled: item.disabled || item.loading,
  color: item.tone === "danger" ? "error" as const : undefined,
  onSelect: item.onSelect,
}))));

function activate(item: AdminRowActionItem, event: MouseEvent) {
  if (item.disabled || item.loading) {
    event.preventDefault();
    return;
  }
  item.onSelect?.();
}
</script>

<template>
  <div
    v-if="groups.length"
    class="flex items-center justify-end gap-1"
    role="group"
    :aria-label="label"
    data-admin-row-actions
    :data-presentation="presentation"
  >
    <template v-if="presentation === 'inline'">
      <UTooltip v-for="item in inlineItems" :key="item.id" :text="item.label">
        <UButton
          :to="item.to"
          :target="item.target"
          :rel="item.rel"
          color="neutral"
          variant="ghost"
          size="xs"
          square
          :disabled="item.disabled"
          :loading="item.loading"
          class="inline-flex size-11 shrink-0 items-center justify-center p-0 sm:size-6"
          :class="item.tone === 'danger' ? 'text-muted hover:text-error focus-visible:text-error' : ''"
          :aria-label="item.label"
          data-admin-row-action
          @click="activate(item, $event)"
        >
          <UIcon :name="item.icon" class="block size-4 shrink-0" aria-hidden="true" />
        </UButton>
      </UTooltip>
    </template>

    <UDropdownMenu v-else :items="menuItems">
      <UButton
        color="neutral"
        variant="ghost"
        size="xs"
        square
        class="inline-flex size-11 shrink-0 items-center justify-center p-0 sm:size-8"
        :aria-label="label"
        data-admin-row-action-overflow
      >
        <UIcon name="i-tabler-dots-vertical" class="block size-4 shrink-0" aria-hidden="true" />
      </UButton>
    </UDropdownMenu>
  </div>
</template>
