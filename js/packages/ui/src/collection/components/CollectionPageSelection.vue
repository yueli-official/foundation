<script setup lang="ts">
const {
  label = "选择当前页",
  disabled = false,
  indeterminate = false,
} = defineProps<{
  label?: string;
  disabled?: boolean;
  indeterminate?: boolean;
}>();
const selected = defineModel<boolean>({ required: true });
</script>
<template>
  <UCheckbox
    v-model="selected"
    :label="label"
    :disabled="disabled"
    :indeterminate="indeterminate"
    :aria-label="label"
    class="shrink-0"
  />
</template>
