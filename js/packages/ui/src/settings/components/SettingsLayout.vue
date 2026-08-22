<script setup lang="ts">
import { computed } from "vue";

export interface SettingsSectionItem {
  readonly key: string;
  readonly label: string;
  readonly description?: string;
  readonly icon?: string;
}

export type SettingsNavigationLayout = "auto" | "sidebar" | "tabs";

const props = withDefaults(
  defineProps<{
    title: string;
    description?: string;
    sections?: readonly SettingsSectionItem[];
    showSectionNavigation?: boolean;
    showHeader?: boolean;
    navigationLabel: string;
    navigationLayout?: SettingsNavigationLayout;
    reserveSaveDock?: boolean;
  }>(),
  {
    description: "",
    sections: () => [],
    showSectionNavigation: true,
    showHeader: true,
    navigationLayout: "auto",
    reserveSaveDock: true,
  },
);
const activeSection = defineModel<string>("activeSection", { default: "" });
const sectionItems = computed(() =>
  props.sections.map((section) => ({
    label: section.label,
    value: section.key,
  })),
);
const useSidebarNavigation = computed(
  () =>
    props.navigationLayout === "sidebar" ||
    (props.navigationLayout === "auto" && props.sections.length >= 5),
);
function selectSection(key: string) {
  activeSection.value = key;
}
</script>

<template>
  <div class="space-y-5" :class="reserveSaveDock ? 'pb-28' : 'pb-8'">
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
      v-if="showSectionNavigation && useSidebarNavigation"
      class="grid min-w-0 gap-4 lg:grid-cols-[12rem_minmax(0,1fr)] lg:gap-6"
      data-settings-navigation-layout="sidebar"
    >
      <aside class="min-w-0 lg:sticky lg:top-5 lg:self-start">
        <USelect
          v-model="activeSection"
          :items="sectionItems"
          value-key="value"
          :aria-label="navigationLabel"
          class="w-full lg:hidden"
        />
        <nav
          class="hidden gap-1 rounded-xl border border-muted bg-elevated p-2 shadow-sm dark:shadow-none lg:grid"
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
            :class="
              activeSection === section.key
                ? 'font-semibold text-highlighted'
                : ''
            "
            @click="selectSection(section.key)"
          />
        </nav>
      </aside>
      <div class="min-w-0 space-y-4"><slot name="notice" /><slot /></div>
    </div>

    <template v-else>
      <div
        v-if="showSectionNavigation && sections.length > 1"
        class="rounded-xl border border-muted bg-elevated p-2 shadow-sm dark:shadow-none"
        data-settings-navigation-layout="tabs"
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
            :class="
              activeSection === section.key
                ? 'font-semibold text-highlighted'
                : ''
            "
            @click="selectSection(section.key)"
          />
        </nav>
      </div>
      <slot name="notice" />
      <slot />
    </template>
  </div>
</template>
