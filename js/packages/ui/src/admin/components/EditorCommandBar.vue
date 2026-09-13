<script setup lang="ts">
withDefaults(
  defineProps<{
    title?: string;
    backTo: string;
    backLabel?: string;
    settingsLabel?: string;
    controlsDisabled?: boolean;
  }>(),
  {
    title: "编辑内容",
    backLabel: "返回列表",
    settingsLabel: "内容设置",
    controlsDisabled: false,
  },
);
const immersive = defineModel<boolean>("immersive", { default: false });
const settingsOpen = defineModel<boolean>("settingsOpen", { default: false });
function toggleImmersive(): void {
  immersive.value = !immersive.value;
}
function toggleSettings(): void {
  settingsOpen.value = !settingsOpen.value;
}
</script>

<template>
  <header
    class="sticky top-0 z-30 grid min-h-16 grid-cols-1 sm:grid-cols-[minmax(0,1fr)_auto] items-center gap-2 border-b border-default bg-default px-3 py-2 sm:gap-4 sm:px-4 lg:px-8"
    data-editor-commandbar
  >
    <div class="flex min-w-0 items-center gap-2">
      <UDashboardSidebarToggle class="size-8 shrink-0 lg:hidden" />
      <UTooltip :text="backLabel">
        <UButton
          :to="backTo"
          :aria-label="backLabel"
          icon="i-tabler-arrow-left"
          color="neutral"
          variant="ghost"
          square
          class="size-8 shrink-0"
        />
      </UTooltip>
      <slot name="title"
        ><span class="truncate text-sm font-semibold text-highlighted">{{
          title
        }}</span></slot
      >
      <slot name="status" />
    </div>
    <div
      class="flex shrink-0 items-center justify-end gap-1 sm:gap-1.5"
      data-editor-command-actions
    >
      <UTooltip :text="immersive ? '退出沉浸模式' : '沉浸模式'">
        <UButton
          :icon="immersive ? 'i-tabler-minimize' : 'i-tabler-maximize'"
          :aria-label="immersive ? '退出沉浸模式' : '沉浸模式'"
          :aria-pressed="immersive"
          :disabled="controlsDisabled"
          color="neutral"
          :variant="immersive ? 'soft' : 'ghost'"
          square
          class="size-8"
          @click="toggleImmersive"
        />
      </UTooltip>
      <slot name="preview" />
      <UTooltip :text="settingsLabel">
        <UButton
          icon="i-tabler-adjustments-horizontal"
          :aria-label="settingsLabel"
          :aria-pressed="settingsOpen"
          :disabled="controlsDisabled"
          :color="settingsOpen ? 'primary' : 'neutral'"
          :variant="settingsOpen ? 'soft' : 'ghost'"
          square
          class="size-8"
          @click="toggleSettings"
        />
      </UTooltip>
      <slot name="lifecycle" />
      <slot name="actions" />
    </div>
  </header>
</template>
