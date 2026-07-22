<script setup lang="ts">
import { computed } from "vue";
const { ascendingLabel = "升序", descendingLabel = "降序" } = defineProps<{
  ascendingLabel?: string;
  descendingLabel?: string;
}>();
const direction = defineModel<"asc" | "desc">({ required: true });
const label = computed(() =>
  direction.value === "asc"
    ? `当前${ascendingLabel}，点击切换为${descendingLabel}`
    : `当前${descendingLabel}，点击切换为${ascendingLabel}`,
);
function toggle() {
  direction.value = direction.value === "asc" ? "desc" : "asc";
}
</script>
<template>
  <UTooltip :text="label"
    ><UButton
      :icon="
        direction === 'asc'
          ? 'i-tabler-sort-ascending'
          : 'i-tabler-sort-descending'
      "
      :aria-label="label"
      color="neutral"
      variant="outline"
      size="xs"
      square
      @click="toggle"
  /></UTooltip>
</template>
