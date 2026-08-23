<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string;
    icon?: string;
    description?: string;
    headingId?: string;
    size?: "default" | "compact";
  }>(),
  { size: "default" },
);
</script>

<template>
  <header
    class="flex flex-wrap items-start justify-between gap-4 sm:items-center sm:gap-6"
    data-manage-page-header
  >
    <div class="flex min-w-0 items-center gap-3.5">
      <span
        v-if="icon"
        class="grid size-10 shrink-0 place-items-center rounded-xl bg-primary/10 text-primary ring-1 ring-primary/20 sm:size-11"
        data-manage-page-icon
      >
        <UIcon :name="icon" class="size-5" />
      </span>
      <div class="min-w-0">
        <div v-if="$slots.eyebrow" class="mb-2"><slot name="eyebrow" /></div>
        <h1
          :id="headingId"
          class="font-display font-semibold tracking-[-0.025em] text-highlighted"
          :class="
            size === 'compact' ? 'text-xl sm:text-2xl' : 'text-2xl sm:text-3xl'
          "
        >
          {{ title }}
        </h1>
        <div v-if="$slots.subtitle" class="mt-1 text-sm leading-6 text-muted">
          <slot name="subtitle" />
        </div>
        <p
          v-else-if="description"
          class="mt-1 max-w-3xl text-sm leading-6 text-muted"
        >
          {{ description }}
        </p>
      </div>
    </div>
    <div
      v-if="$slots.actions"
      class="flex shrink-0 flex-wrap items-center gap-2"
      data-manage-page-actions
    >
      <slot name="actions" />
    </div>
  </header>
</template>
