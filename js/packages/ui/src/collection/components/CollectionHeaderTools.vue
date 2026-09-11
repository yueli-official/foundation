<script setup lang="ts">
import { computed, ref } from "vue";
import type { CollectionControl, CollectionControlValue } from "../panel";
import CollectionTableToolbar from "./CollectionTableToolbar.vue";
import CollectionViewToggle from "./CollectionViewToggle.vue";
const props = withDefaults(defineProps<{
  label: string; searchPlaceholder: string; searchId?: string; searchable?: boolean; controls?: readonly CollectionControl[]; sortControls?: readonly CollectionControl[]; filterCount?: number;
  sortBy?: string; sortOrder?: "asc" | "desc"; sortOptions?: readonly {label: string; value: string}[];
  view?: "list" | "grid" | "tree" | "table"; viewItems?: readonly {key: "list" | "grid" | "tree" | "table"; label: string; icon: string}[];
}>(), { searchable: true });
const search = defineModel<string>("search", {default: ""});
const emit = defineEmits<{
  controlChange: [id: string, value: CollectionControlValue];
  search: [value: string]; filters: [values: Record<string, CollectionControlValue>];
  sort: [by: string, order: "asc" | "desc"]; "update:view": [value: "list" | "grid" | "tree" | "table"];
}>();
const filters = computed(() => (props.controls ?? []).filter(c => c.kind === "select"));
const filtersOpen = ref(false); const sortOpen = ref(false);
const draft = ref<Record<string, CollectionControlValue>>({});
const controlSortDraft = ref<Record<string, CollectionControlValue>>({});
const sortDraft = ref(""); const directionDraft = ref<"asc" | "desc">("desc");
function openFilters() { draft.value = Object.fromEntries(filters.value.map(c => [c.id, c.value])); filtersOpen.value = true; }
function openSort() { controlSortDraft.value = Object.fromEntries((props.sortControls ?? []).map(c => [c.id, c.value])); sortDraft.value = props.sortBy ?? ""; directionDraft.value = props.sortOrder ?? "desc"; sortOpen.value = true; }
function applyFilters() { emit("filters", {...draft.value}); for (const control of filters.value) { if (draft.value[control.id] !== control.value) emit("controlChange", control.id, draft.value[control.id]!); } filtersOpen.value = false; }
function applySort() { if (props.sortControls?.length) { for (const control of props.sortControls) { if (controlSortDraft.value[control.id] !== control.value) emit("controlChange", control.id, controlSortDraft.value[control.id]!); } } else emit("sort", sortDraft.value, directionDraft.value); sortOpen.value = false; }
</script>
<template>
  <CollectionTableToolbar v-model:search="search" presentation="header" :searchable="searchable" :search-id="searchId" :label="label" :search-placeholder="searchPlaceholder" filter-label="筛选" @search="emit('search', $event)">
    <template #utilities>
      <UButton v-if="filters.length" color="neutral" variant="outline" icon="i-tabler-adjustments-horizontal" :label="filterCount ? `筛选 · ${filterCount}` : '筛选'" class="h-9" @click="openFilters" />
      <UButton v-if="sortOptions?.length || sortControls?.length" color="neutral" variant="outline" icon="i-tabler-sort-descending" label="排序" class="h-9" @click="openSort" />
      <CollectionViewToggle v-if="viewItems?.length" appearance="surface" :model-value="view ?? 'list'" :items="[...viewItems]" @update:model-value="emit('update:view', $event)" />
      <slot name="view" />
    </template>
  </CollectionTableToolbar>
  <UModal v-model:open="filtersOpen" title="筛选" :ui="{content: 'max-w-sm', footer: 'justify-end'}">
    <template #body><div class="grid gap-4">
      <UFormField v-for="control in filters" :key="control.id" :label="control.label">
        <USelectMenu v-if="control.options.length > 10" v-model="draft[control.id]" :items="[...control.options]" value-key="value" :aria-label="control.label" class="w-full" />
        <USelect v-else v-model="draft[control.id]" :items="[...control.options]" :aria-label="control.label" class="w-full" />
      </UFormField>
    </div></template>
    <template #footer><UButton label="取消" color="neutral" variant="outline" @click="void (filtersOpen = false)" /><UButton label="应用筛选" @click="applyFilters" /></template>
  </UModal>
  <UModal v-model:open="sortOpen" title="排序" :ui="{content: 'max-w-sm', footer: 'justify-end'}">
    <template #body><div class="grid gap-5">
      <template v-if="sortControls?.length">
        <UFormField v-for="control in sortControls" :key="control.id" :label="control.label">
          <USelect v-if="control.kind === 'select'" v-model="controlSortDraft[control.id]" :items="[...control.options]" class="w-full" />
          <URadioGroup v-else :model-value="String(controlSortDraft[control.id])" @update:model-value="controlSortDraft[control.id] = $event" :items="[{label:'升序',value:'asc'},{label:'降序',value:'desc'}]" />
        </UFormField>
      </template>
      <template v-else><UFormField label="排序依据"><URadioGroup v-model="sortDraft" :items="[...(sortOptions ?? [])]" /></UFormField>
      <UFormField label="顺序"><URadioGroup v-model="directionDraft" :items="[{label:'升序',value:'asc'},{label:'降序',value:'desc'}]" /></UFormField></template>
    </div></template>
    <template #footer><UButton label="取消" color="neutral" variant="outline" @click="void (sortOpen = false)" /><UButton label="应用排序" @click="applySort" /></template>
  </UModal>
</template>
