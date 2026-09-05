<script setup lang="ts">
import type { MobileBottomNavItem } from "../mobile-bottom-nav.types";

withDefaults(
  defineProps<{
    items: readonly MobileBottomNavItem[];
    label: string;
    /** Remove navigation and its occupied space together. */
    hidden?: boolean;
  }>(),
  { hidden: false },
);
const emit = defineEmits<{ select: [item: MobileBottomNavItem] }>();
</script>

<template>
  <template v-if="!hidden && items.length">
    <div data-mobile-bottom-spacer aria-hidden="true" class="shrink-0" />
    <nav
      :aria-label="label"
      data-mobile-bottom-nav
      data-y-dock
      class="fixed inset-x-0 bottom-0 z-30 border-t border-default bg-default"
    >
      <div class="flex h-12 items-stretch">
        <ULink
          v-for="item in items"
          :key="item.id"
          :as="item.to ? 'a' : 'button'"
          :to="item.to"
          :type="item.to ? undefined : 'button'"
          :active="Boolean(item.active)"
          :aria-current="item.active ? 'page' : undefined"
          :aria-label="item.ariaLabel"
          :disabled="item.disabled"
          raw
          class="flex min-w-0 flex-1 flex-col items-center justify-center gap-0.5 text-[11px] leading-3.5 no-underline outline-offset-[-3px] focus-visible:outline-2 focus-visible:outline-primary"
          :class="[
            item.active
              ? 'font-semibold text-primary'
              : 'text-muted hover:text-highlighted',
            item.disabled ? 'cursor-not-allowed opacity-50' : '',
          ]"
          @click="!item.disabled && emit('select', item)"
        >
          <span class="relative flex">
            <UIcon :name="item.icon" class="size-5" aria-hidden="true" />
            <span
              v-if="item.badge !== undefined && item.badge !== ''"
              class="absolute -end-2 -top-1 min-w-3.5 rounded-full bg-primary px-1 text-center text-[10px] leading-3.5 text-inverted"
              :aria-hidden="item.ariaLabel ? true : undefined"
              >{{ item.badge }}</span
            >
          </span>
          <span class="max-w-full truncate px-1">{{ item.label }}</span>
        </ULink>
      </div>
    </nav>
  </template>
</template>
