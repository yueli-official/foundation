<script setup lang="ts">
withDefaults(
  defineProps<{ title: string; description?: string; danger?: boolean }>(),
  { description: "", danger: false },
);
</script>

<template>
  <section
    class="min-w-0 overflow-hidden rounded-xl border bg-default shadow-sm dark:shadow-none"
    :class="danger ? 'border-error/40' : 'border-muted'"
    data-setting-section
  >
    <header class="flex min-w-0 items-start justify-between gap-4 border-b border-default bg-elevated/35 px-4 py-3 sm:px-5">
      <div class="min-w-0">
        <h2
          class="text-sm font-semibold"
          :class="danger ? 'text-error' : 'text-highlighted'"
        >
          {{ title }}
        </h2>
        <p v-if="description" class="mt-1 text-sm leading-6 text-muted">
          {{ description }}
        </p>
        <slot name="aside" />
      </div>
      <div v-if="$slots.actions" class="flex shrink-0 items-center gap-2" data-setting-section-actions>
        <slot name="actions" />
      </div>
    </header>
    <div class="min-w-0 p-4 sm:p-5" data-setting-section-body><slot /></div>
  </section>
</template>
