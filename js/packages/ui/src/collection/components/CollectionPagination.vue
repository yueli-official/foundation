<script setup lang="ts">
const props = defineProps<{ totalPages: number }>();
const page = defineModel<number>({ required: true });
function go(value: number) {
  if (value >= 1 && value <= props.totalPages) page.value = value;
}
</script>

<template>
  <UPagination
    v-if="totalPages > 1"
    :page="page"
    :total="totalPages"
    :items-per-page="1"
    :show-edges="false"
    :sibling-count="1"
    size="xs"
    @update:page="go"
  />
</template>
