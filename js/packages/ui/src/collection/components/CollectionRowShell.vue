<script setup lang="ts">
const { selected = false, selectionDisabled = false } = defineProps<{
  selected?: boolean;
  selectionDisabled?: boolean;
  selectionLabel: string;
}>();
const emit = defineEmits<{ select: [selected: boolean] }>();
</script>
<template>
  <article
    class="group relative grid min-w-0 grid-cols-[2.5rem_minmax(0,1fr)_auto] items-center border-b border-default bg-default transition-colors last:border-b-0 md:grid-cols-[2.5rem_auto_minmax(0,1fr)_auto_auto]"
    :class="
      selected
        ? 'bg-primary/5 before:absolute before:inset-y-0 before:left-0 before:w-0.5 before:bg-primary'
        : 'hover:bg-elevated/40 focus-within:bg-elevated/40'
    "
  >
    <div class="grid min-h-20 place-items-center pl-2">
      <UCheckbox
        :model-value="selected"
        :disabled="selectionDisabled"
        :aria-label="selectionLabel"
        @update:model-value="emit('select', $event === true)"
      />
    </div>
    <div v-if="$slots.media" class="hidden shrink-0 py-3 md:block">
      <slot name="media" />
    </div>
    <div class="min-w-0 px-3 py-3 md:px-4"><slot /></div>
    <div
      v-if="$slots.meta"
      class="col-span-2 col-start-2 min-w-0 px-3 pb-3 md:col-auto md:col-span-1 md:px-4 md:py-3"
    >
      <slot name="meta" />
    </div>
    <div
      v-if="$slots.actions"
      class="col-start-3 row-start-1 flex items-center gap-1 pr-2 md:col-auto md:row-auto md:pr-3"
    >
      <slot name="actions" />
    </div>
  </article>
</template>
