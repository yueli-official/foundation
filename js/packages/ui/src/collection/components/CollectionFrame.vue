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
    class="@container/collection overflow-clip rounded-xl border border-default bg-default"
  >
    <slot v-if="$slots.toolbar" name="toolbar" />
    <template v-else>
      <div class="border-b border-default p-3 sm:p-4">
        <slot
          name="search"
          :controls-id="controlsId"
          :controls-open="controlsOpen"
          :toggle-controls="toggleControls"
        />

        <div
          v-if="$slots.controls"
          :id="controlsId"
          :class="[
            controlsOpen ? 'flex' : 'hidden sm:flex',
            'mt-3 flex-wrap items-center gap-2',
          ]"
        >
          <slot name="controls" />
        </div>
      </div>

      <div
        v-if="bulkVisible"
        data-collection-bulk
        role="region"
        :aria-label="bulkLabel"
        class="flex min-h-11 items-center justify-between gap-3 overflow-x-auto border-b border-default bg-primary/5 px-3 py-2 sm:px-4"
      >
        <slot name="bulk" />
      </div>
    </template>
    <div
      v-if="$slots.columns"
      data-collection-columns
      class="flex min-h-10 items-center border-b border-default px-3 py-2 sm:px-4"
    >
      <slot name="columns" />
    </div>

    <div aria-live="polite">
      <slot />
    </div>

    <footer
      v-if="$slots.footer"
      class="border-t border-default px-3 py-3 sm:px-4"
    >
      <slot name="footer" />
    </footer>
  </section>
</template>
