<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string;
    description?: string;
    headingId?: string;
    size?: "default" | "compact";
  }>(),
  { size: "default" },
);
</script>

<template>
  <header
    class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between"
  >
    <div class="min-w-0">
      <div v-if="$slots.eyebrow" class="mb-2"><slot name="eyebrow" /></div>
      <h1
        :id="headingId"
        class="font-semibold tracking-tight text-highlighted"
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
    <div v-if="$slots.actions" class="shrink-0"><slot name="actions" /></div>
  </header>
</template>
