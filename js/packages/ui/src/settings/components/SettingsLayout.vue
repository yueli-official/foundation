<script setup lang="ts">
import { computed } from "vue";

export interface SettingsSectionItem {
  readonly key: string;
  readonly label: string;
  readonly description?: string;
  readonly icon?: string;
}

const props = withDefaults(
  defineProps<{
    title: string;
    description?: string;
    sections?: readonly SettingsSectionItem[];
    showSectionNavigation?: boolean;
    showHeader?: boolean;
    navigationLabel: string;
  }>(),
  {
    description: "",
    sections: () => [],
    showSectionNavigation: true,
    showHeader: true,
  },
);
const activeSection = defineModel<string>("activeSection", { default: "" });
const sectionItems = computed(() =>
  props.sections.map((section) => ({
    label: section.label,
    value: section.key,
  })),
);
function selectSection(key: string) {
  activeSection.value = key;
}
</script>

<template>
  <div class="space-y-5 pb-28">
    <div
      v-if="showHeader"
      class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between"
    >
      <div class="min-w-0">
        <h1 class="text-2xl font-semibold tracking-tight text-highlighted">
          {{ title }}
        </h1>
        <p
          v-if="description"
          class="mt-1 max-w-3xl text-sm leading-6 text-muted"
        >
          {{ description }}
        </p>
      </div>
      <div v-if="$slots.actions" class="shrink-0"><slot name="actions" /></div>
    </div>

    <div
      v-if="showSectionNavigation && sections.length >= 5"
      class="grid min-w-0 gap-5 lg:grid-cols-[14rem_minmax(0,1fr)]"
    >
      <aside class="min-w-0 lg:sticky lg:top-20 lg:self-start">
        <USelect
          v-model="activeSection"
          :items="sectionItems"
          value-key="value"
          :aria-label="navigationLabel"
          class="w-full lg:hidden"
        />
        <nav
          class="hidden gap-1 rounded-lg border border-default bg-default p-2 lg:grid"
          :aria-label="navigationLabel"
        >
          <UButton
            v-for="section in sections"
            :key="section.key"
            :icon="section.icon"
            :label="section.label"
            :color="activeSection === section.key ? 'primary' : 'neutral'"
            :variant="activeSection === section.key ? 'soft' : 'ghost'"
            class="justify-start"
            @click="selectSection(section.key)"
          />
        </nav>
      </aside>
      <div class="min-w-0 space-y-5"><slot name="notice" /><slot /></div>
    </div>

    <template v-else>
      <div
        v-if="showSectionNavigation && sections.length > 1"
        class="rounded-lg border border-default bg-default p-2"
      >
        <USelect
          v-model="activeSection"
          :items="sectionItems"
          value-key="value"
          :aria-label="navigationLabel"
          class="w-full md:hidden"
        />
        <nav class="hidden gap-1 md:flex" :aria-label="navigationLabel">
          <UButton
            v-for="section in sections"
            :key="section.key"
            :icon="section.icon"
            :label="section.label"
            :color="activeSection === section.key ? 'primary' : 'neutral'"
            :variant="activeSection === section.key ? 'soft' : 'ghost'"
            @click="selectSection(section.key)"
          />
        </nav>
      </div>
      <slot name="notice" />
      <slot />
    </template>
  </div>
</template>
