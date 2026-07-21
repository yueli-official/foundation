<script setup lang="ts">
import { useId } from "vue";

defineOptions({ inheritAttrs: false });

const {
  label,
  labelledby,
  bulkLabel,
  bulkVisible = false,
} = defineProps<{
  /** Accessible label used when no visible heading labels the frame. */
  label?: string;
  /** ID of a caller-owned visible heading. Preferred over `label`. */
  labelledby?: string;
  /** Caller-owned accessible label for the bulk-action region. */
  bulkLabel?: string;
  /** Keeps the bulk region out of the document when no selection exists. */
  bulkVisible?: boolean;
}>();

const controlsOpen = defineModel<boolean>("controlsOpen", { default: false });
const controlsId = `y-collection-controls-${useId().replaceAll(":", "")}`;

function toggleControls() {
  controlsOpen.value = !controlsOpen.value;
}
</script>

<template>
  <section
    v-bind="$attrs"
    :aria-label="labelledby ? undefined : label"
    :aria-labelledby="labelledby"
    class="@container/collection overflow-clip rounded-xl border border-default bg-default shadow-sm"
  >
    <div class="border-b border-default p-3 sm:p-4">
      <slot
        name="search"
        :controls-id="controlsId"
        :controls-open="controlsOpen"
        :toggle-controls="toggleControls"
      />

      <div
        :id="controlsId"
        :class="[
          controlsOpen ? 'flex' : 'hidden',
          'mt-3 flex-wrap items-center gap-2 @sm/collection:flex',
        ]"
      >
        <slot name="controls" />
      </div>
    </div>

    <div
      v-if="bulkVisible"
      role="region"
      :aria-label="bulkLabel"
      class="sticky top-2 z-20 mx-3 mt-3 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-primary/25 bg-default/95 px-3 py-2 shadow-lg backdrop-blur motion-reduce:backdrop-blur-none sm:mx-4"
    >
      <slot name="bulk" />
    </div>

    <div class="border-b border-default bg-muted/40 px-3 py-2 sm:px-4">
      <slot name="columns" />
    </div>

    <div aria-live="polite">
      <slot />
    </div>

    <footer class="border-t border-default bg-muted/20 px-3 py-3 sm:px-4">
      <slot name="footer" />
    </footer>
  </section>
</template>
