<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  label: string;
  active: boolean;
  sortOrder: "asc" | "desc";
}>();

const emit = defineEmits<{ sort: [] }>();
const icon = computed(() =>
  props.active
    ? props.sortOrder === "asc"
      ? "i-tabler-arrow-up"
      : "i-tabler-arrow-down"
    : "i-tabler-arrows-sort",
);
const actionLabel = computed(() =>
  props.active
    ? `${props.label}，当前${props.sortOrder === "asc" ? "正序" : "倒序"}，点击切换为${props.sortOrder === "asc" ? "倒序" : "正序"}`
    : `按${props.label}排序`,
);
</script>

<template>
  <UButton
    :label="label"
    :icon="icon"
    :aria-label="actionLabel"
    :aria-pressed="active"
    color="neutral"
    variant="ghost"
    size="xs"
    class="-mx-2 min-h-8 justify-start px-2"
    @click="emit('sort')"
  />
</template>
