<script setup lang="ts">
export interface CollectionLifecycleTab {
  key: string;
  label: string;
  count?: number;
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
    class="overflow-x-auto overflow-y-hidden border-b border-default [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
  >
    <div class="flex min-w-max items-center gap-1">
      <UButton
        v-for="item in items"
        :key="item.key"
        color="neutral"
        variant="ghost"
        size="sm"
        class="-mb-px rounded-none border-b-2 px-3 py-2.5 font-medium"
        :class="
          model === item.key
            ? 'border-primary text-primary'
            : 'border-transparent text-muted hover:text-default'
        "
        :aria-current="model === item.key ? 'page' : undefined"
        @click="select(item.key)"
      >
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
  </nav>
</template>
