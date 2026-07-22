<script setup lang="ts">
withDefaults(
  defineProps<{ title: string; description?: string; danger?: boolean }>(),
  { description: "", danger: false },
);
</script>

<template>
  <section
    class="grid min-w-0 gap-4 rounded-lg border bg-default p-4 sm:p-5 lg:grid-cols-[minmax(9rem,13rem)_minmax(0,1fr)] lg:gap-8"
    :class="danger ? 'border-error/40' : 'border-default'"
  >
    <header class="min-w-0">
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
    </header>
    <div class="min-w-0"><slot /></div>
  </section>
</template>
