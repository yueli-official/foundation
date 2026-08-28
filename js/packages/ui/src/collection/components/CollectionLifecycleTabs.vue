<script setup lang="ts">
export interface CollectionLifecycleTab {
  key: string;
  label: string;
  count?: number;
  icon?: string;
}
defineProps<{ items: readonly CollectionLifecycleTab[]; label?: string }>();
const model = defineModel<string>({ required: true });
function select(key: string) {
  model.value = key;
}
</script>
<template>
  <nav
    :aria-label="label || '内容状态'"
    class="flex min-w-0 items-end justify-between gap-3 border-b border-default px-3 sm:px-4"
  >
    <div
      class="min-w-0 flex-1 overflow-x-auto overflow-y-hidden [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    >
      <div class="flex min-w-max items-center gap-1">
        <UButton
          v-for="item in items"
          :key="item.key"
          color="neutral"
          variant="ghost"
          size="sm"
          class="-mb-px min-h-11 rounded-none border-b-2 px-3 py-3 font-medium"
          :class="
            model === item.key
              ? 'border-primary text-primary'
              : 'border-transparent text-muted hover:text-default'
          "
          :aria-current="model === item.key ? 'page' : undefined"
          @click="select(item.key)"
        >
          <UIcon
            v-if="item.icon"
            :name="item.icon"
            class="size-4 shrink-0"
            aria-hidden="true"
          />
          <span>{{ item.label }}</span
          ><span
            v-if="item.count != null"
            class="rounded-full px-1.5 py-0.5 text-xs tabular-nums"
            :class="
              model === item.key
                ? 'bg-primary/10 text-primary'
                : 'bg-elevated text-dimmed'
            "
            >{{ item.count }}</span
          >
        </UButton>
      </div>
    </div>
    <div v-if="$slots.actions" class="shrink-0 pb-2">
      <slot name="actions" />
    </div>
  </nav>
</template>
