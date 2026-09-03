<script setup lang="ts">
import { addCollection } from "@iconify/vue";
import { computed, onMounted, ref, shallowRef } from "vue";
import { ADMIN_TABLER_ICON_OPTIONS, filterAdminIconOptions } from "../icons";
import type { AdminIconOption } from "../types";

const props = withDefaults(
  defineProps<{
    modelValue?: string;
    options?: readonly AdminIconOption[];
    disabled?: boolean;
    compact?: boolean;
    fullCatalog?: boolean;
    resultLimit?: number;
    searchPlaceholder?: string;
    emptyLabel?: string;
  }>(),
  {
    modelValue: "",
    options: () => [],
    disabled: false,
    compact: false,
    fullCatalog: true,
    resultLimit: 96,
    searchPlaceholder: "搜索全部 Tabler 图标",
    emptyLabel: "没有匹配的图标",
  },
);
const emit = defineEmits<{ "update:modelValue": [value: string] }>();
const search = ref("");
const fullCatalogItems = shallowRef<readonly AdminIconOption[]>([]);
const catalogLoading = ref(false);
const catalogError = ref(false);
const items = computed(() =>
  props.options.length ? props.options : ADMIN_TABLER_ICON_OPTIONS,
);
const filteredItems = computed(() =>
  filterAdminIconOptions(
    search.value && fullCatalogItems.value.length
      ? fullCatalogItems.value
      : items.value,
    search.value,
  ).slice(0, props.resultLimit),
);
const selected = computed(
  () => props.modelValue || items.value[0]?.value || "i-tabler-apps",
);

function choose(value: string) {
  if (!props.disabled) emit("update:modelValue", value);
}

async function loadFullCatalog() {
  if (
    !props.fullCatalog ||
    props.options.length ||
    fullCatalogItems.value.length ||
    catalogLoading.value
  )
    return;
  catalogLoading.value = true;
  catalogError.value = false;
  try {
    const module = await import("@iconify-json/tabler/icons.json");
    const collection = module.default;
    addCollection(collection);
    fullCatalogItems.value = Object.keys(collection.icons)
      .sort((left, right) => left.localeCompare(right))
      .map((name) => {
        const value = `i-tabler-${name}`;
        const curated = ADMIN_TABLER_ICON_OPTIONS.find(
          (item) => item.value === value,
        );
        return {
          label: curated?.label || name,
          value,
          keywords: curated?.keywords || name.split("-"),
        };
      });
  } catch {
    catalogError.value = true;
  } finally {
    catalogLoading.value = false;
  }
}

onMounted(loadFullCatalog);
</script>

<template>
  <div
    class="grid min-w-0 gap-2"
    :class="compact ? 'w-56 max-w-[calc(100vw-2rem)]' : 'w-full'"
    data-admin-icon-picker
  >
    <div
      v-if="!compact"
      class="flex min-w-0 items-center justify-between gap-3"
    >
      <p class="text-xs font-medium text-muted">图标</p>
      <span class="truncate font-mono text-xs text-muted">{{ selected }}</span>
    </div>
    <UInput
      v-model="search"
      icon="i-tabler-search"
      :placeholder="searchPlaceholder"
      aria-label="搜索图标"
      size="sm"
      class="w-full"
      :disabled="disabled"
      data-admin-icon-search
    />
    <div
      v-if="filteredItems.length"
      class="grid max-h-56 grid-cols-[repeat(auto-fill,minmax(2rem,1fr))] auto-rows-[2rem] gap-2 overflow-y-auto pr-1"
      data-admin-icon-results
    >
      <UTooltip
        v-for="item in filteredItems"
        :key="item.value"
        :text="item.label"
      >
        <UButton
          :icon="item.value"
          :color="selected === item.value ? 'primary' : 'neutral'"
          :variant="selected === item.value ? 'soft' : 'outline'"
          size="sm"
          square
          class="grid size-8 place-items-center justify-self-center p-0"
          :ui="{ leadingIcon: 'mx-auto size-4 shrink-0' }"
          :aria-label="`选择${item.label}`"
          :aria-pressed="selected === item.value"
          :disabled="disabled"
          @click="choose(item.value)"
        />
      </UTooltip>
    </div>
    <p
      v-else-if="search && catalogLoading"
      class="rounded-lg bg-elevated px-3 py-3 text-center text-xs text-muted"
      role="status"
    >
      正在加载完整 Tabler 图标目录…
    </p>
    <p
      v-else-if="search && catalogError"
      class="rounded-lg bg-elevated px-3 py-3 text-center text-xs text-muted"
      role="status"
    >
      完整目录加载失败，仍可使用常用图标
    </p>
    <p
      v-else
      class="rounded-lg bg-elevated px-3 py-6 text-center text-xs text-muted"
    >
      {{ emptyLabel }}
    </p>
  </div>
</template>
