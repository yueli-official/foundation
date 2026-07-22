<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import type {
  RemoteSelectLoader,
  RemoteSelectMessages,
  RemoteSelectOption,
  RemoteSelectValue,
} from "../types";

defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    load: RemoteSelectLoader;
    messages: RemoteSelectMessages;
    initialItems?: readonly RemoteSelectOption[];
    debounceMs?: number;
    minimumQueryLength?: number;
    cache?: boolean;
    clear?: boolean;
    disabled?: boolean;
    ariaLabel?: string;
  }>(),
  {
    initialItems: () => [],
    debounceMs: 250,
    minimumQueryLength: 0,
    cache: true,
    clear: true,
    disabled: false,
  },
);
const model = defineModel<RemoteSelectValue | null>({ default: null });
const emit = defineEmits<{
  loadError: [error: unknown];
}>();

const open = ref(false);
const searchTerm = ref("");
const loading = ref(false);
const issue = ref<unknown>();
const loadedItems = ref<readonly RemoteSelectOption[]>([]);
const resultCache = new Map<string, readonly RemoteSelectOption[]>();
let timer: ReturnType<typeof setTimeout> | undefined;
let controller: AbortController | undefined;
let sequence = 0;

const normalizedQuery = computed(() => searchTerm.value.trim());
const queryTooShort = computed(
  () => normalizedQuery.value.length < props.minimumQueryLength,
);
const items = computed(() => {
  const merged = new Map<RemoteSelectValue, RemoteSelectOption>();
  for (const item of props.initialItems) merged.set(item.value, item);
  for (const item of loadedItems.value) merged.set(item.value, item);
  return [...merged.values()];
});

function clearTimer() {
  if (timer !== undefined) clearTimeout(timer);
  timer = undefined;
}

function abortLoad() {
  controller?.abort();
  controller = undefined;
}

async function runLoad(query: string, force = false) {
  if (query.length < props.minimumQueryLength) {
    loadedItems.value = [];
    issue.value = undefined;
    loading.value = false;
    abortLoad();
    return;
  }

  if (props.cache && !force && resultCache.has(query)) {
    loadedItems.value = resultCache.get(query) ?? [];
    issue.value = undefined;
    loading.value = false;
    return;
  }

  abortLoad();
  const requestController = new AbortController();
  controller = requestController;
  const requestSequence = ++sequence;
  loadedItems.value = [];
  loading.value = true;
  issue.value = undefined;

  try {
    const result = await props.load({
      query,
      signal: requestController.signal,
    });
    if (requestController.signal.aborted || requestSequence !== sequence)
      return;
    const nextItems = [...result.items];
    loadedItems.value = nextItems;
    if (props.cache) resultCache.set(query, nextItems);
  } catch (error) {
    if (requestController.signal.aborted || requestSequence !== sequence)
      return;
    issue.value = error;
    emit("loadError", error);
  } finally {
    if (requestSequence === sequence) {
      loading.value = false;
      if (controller === requestController) controller = undefined;
    }
  }
}

function scheduleLoad() {
  clearTimer();
  if (!open.value) return;
  timer = setTimeout(
    () => void runLoad(normalizedQuery.value),
    props.debounceMs,
  );
}

function setOpen(value: boolean) {
  open.value = value;
  if (value) scheduleLoad();
  else {
    clearTimer();
    abortLoad();
  }
}

function retry() {
  clearTimer();
  resultCache.delete(normalizedQuery.value);
  void runLoad(normalizedQuery.value, true);
}

watch(searchTerm, scheduleLoad);
onBeforeUnmount(() => {
  clearTimer();
  abortLoad();
});

defineExpose({ refresh: retry });
</script>

<template>
  <USelectMenu
    v-bind="$attrs"
    v-model="model"
    v-model:search-term="searchTerm"
    :open="open"
    :items="items"
    value-key="value"
    label-key="label"
    ignore-filter
    :clear="clear"
    :disabled="disabled"
    :placeholder="messages.placeholder"
    :aria-label="ariaLabel"
    :search-input="{
      placeholder: messages.searchPlaceholder,
      loading,
    }"
    @update:open="setOpen"
  >
    <template v-if="$slots.leading" #leading="slotProps">
      <slot name="leading" v-bind="slotProps" />
    </template>
    <template v-if="$slots.trailing" #trailing="slotProps">
      <slot name="trailing" v-bind="slotProps" />
    </template>
    <template #item="slotProps">
      <slot name="item" v-bind="slotProps">
        <div class="min-w-0">
          <p class="truncate text-sm text-highlighted">
            {{ slotProps.item.label }}
          </p>
          <p
            v-if="slotProps.item.description"
            class="truncate text-xs text-muted"
          >
            {{ slotProps.item.description }}
          </p>
        </div>
      </slot>
    </template>
    <template #empty>
      <div
        v-if="issue"
        class="flex items-center justify-between gap-3 px-2 py-1"
        role="status"
      >
        <span class="text-sm text-error">{{ messages.error }}</span>
        <UButton
          :label="messages.retry"
          color="neutral"
          variant="ghost"
          size="xs"
          @click="retry"
        />
      </div>
      <p v-else class="px-2 py-1 text-sm text-muted" role="status">
        {{
          queryTooShort
            ? messages.minimumQuery(minimumQueryLength)
            : messages.empty
        }}
      </p>
    </template>
  </USelectMenu>
</template>
