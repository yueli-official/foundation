<script setup lang="ts">
import { computed, useSlots } from "vue";
import PageHeader from "../../dashboard/components/PageHeader.vue";

defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    id: string;
    title: string;
    icon: string;
    bodyClass?: string;
  }>(),
  { bodyClass: "" },
);
const slots = useSlots();
const headingId = computed(() => `${props.id}-title`);
</script>

<template>
  <section
    v-bind="$attrs"
    :id="id"
    :aria-labelledby="headingId"
    class="min-w-0 space-y-5"
    data-manage-page
  >
    <PageHeader :title="title" :icon="icon" :heading-id="headingId">
      <template v-if="slots.subtitle" #subtitle
        ><slot name="subtitle"
      /></template>
      <template v-if="slots.actions" #actions><slot name="actions" /></template>
    </PageHeader>

    <div class="min-w-0 space-y-5" :class="bodyClass"><slot /></div>
    <footer v-if="slots.footer"><slot name="footer" /></footer>
  </section>
</template>
