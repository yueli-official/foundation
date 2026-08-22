<script setup lang="ts">
withDefaults(
  defineProps<{ title: string; description?: string; danger?: boolean }>(),
  { description: "", danger: false },
);
</script>

<template>
  <section
    class="grid min-w-0 gap-5 rounded-xl border bg-elevated p-5 shadow-sm dark:shadow-none sm:p-6 lg:grid-cols-[11rem_minmax(0,1fr)] lg:gap-8"
    :class="danger ? 'border-error/40' : 'border-muted'"
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
