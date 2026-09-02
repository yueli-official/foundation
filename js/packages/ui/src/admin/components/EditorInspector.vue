<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";

const props = withDefaults(
  defineProps<{
    title: string;
    description?: string;
  }>(),
  {
    description: undefined,
  },
);

const open = defineModel<boolean>("open", { default: false });
const docked = ref(false);
let media: MediaQueryList | undefined;

function syncMode(event?: MediaQueryListEvent) {
  const next = event?.matches ?? media?.matches ?? false;
  if (open.value && next !== docked.value) open.value = false;
  docked.value = next;
}

function close() {
  open.value = false;
}

onMounted(() => {
  media = window.matchMedia("(min-width: 1280px)");
  syncMode();
  media.addEventListener("change", syncMode);
});

onBeforeUnmount(() => {
  media?.removeEventListener("change", syncMode);
});
</script>

<template>
  <USlideover
    v-model:open="open"
    :title="props.title"
    :description="props.description"
    :modal="!docked"
    :overlay="!docked"
    :dismissible="!docked"
    class="y-editor-inspector-surface"
    :ui="{
      content: docked
        ? 'top-16 z-40 h-[calc(100svh-4rem)] w-[25rem] max-w-[25rem] border-l border-default bg-default shadow-none'
        : 'w-full max-w-md bg-default',
      header: 'border-b border-default bg-default px-5 py-4',
      body: 'bg-default p-5',
      footer: docked ? 'hidden' : 'bg-default',
    }"
  >
    <template #body>
      <div
        class="min-w-0"
        data-y-editor-inspector
        :data-inspector-mode="docked ? 'docked' : 'overlay'"
      >
        <slot :docked="docked" :close="close" />
      </div>
    </template>

    <template #footer>
      <slot name="footer" :docked="docked" :close="close" />
    </template>
  </USlideover>
</template>
