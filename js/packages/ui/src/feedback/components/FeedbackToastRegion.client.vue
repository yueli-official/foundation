<script setup lang="ts">
import { useToast } from "@nuxt/ui/composables";
import { computed, onBeforeUnmount, watch } from "vue";

type ToastTone = "primary" | "secondary" | "success" | "info" | "warning" | "error" | "neutral";

interface ToastNotice {
  id: string | number;
  open?: boolean;
  title?: string;
  description?: string;
  icon?: string;
  color?: ToastTone;
  duration?: number;
  close?: boolean;
  type?: "foreground" | "background";
  _duplicate?: number;
}

const toast = useToast();
const timers = new Map<string | number, ReturnType<typeof setTimeout>>();
const notices = computed(() => (toast.toasts.value as ToastNotice[])
  .filter(notice => notice.open !== false)
  .slice(-3));

function clearTimer(id: string | number) {
  const timer = timers.get(id);
  if (timer) clearTimeout(timer);
  timers.delete(id);
}

function schedule(notice: ToastNotice) {
  clearTimer(notice.id);
  const duration = notice.duration ?? 5000;
  if (duration <= 0) return;
  timers.set(notice.id, setTimeout(() => toast.remove(notice.id), duration));
}

watch(
  () => notices.value.map(notice => `${notice.id}:${notice.duration ?? 5000}:${notice._duplicate ?? 0}`).join("|"),
  () => {
    const visible = new Set(notices.value.map(notice => notice.id));
    for (const id of timers.keys()) if (!visible.has(id)) clearTimer(id);
    for (const notice of notices.value) schedule(notice);
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  for (const id of timers.keys()) clearTimer(id);
});

function tone(notice: ToastNotice) {
  return notice.color ?? "neutral";
}

function role(notice: ToastNotice) {
  return notice.type === "foreground" || tone(notice) === "error" || tone(notice) === "warning"
    ? "alert"
    : "status";
}
</script>

<template>
  <Teleport to="body">
    <TransitionGroup
      tag="section"
      name="yueli-toast"
      class="yueli-toast-region"
      aria-label="通知"
    >
      <article
        v-for="notice in notices"
        :key="notice.id"
        class="yueli-toast-notice"
        :data-tone="tone(notice)"
        :role="role(notice)"
        data-yueli-toast
        @mouseenter="clearTimer(notice.id)"
        @mouseleave="schedule(notice)"
      >
        <UIcon
          v-if="notice.icon"
          :name="notice.icon"
          class="yueli-toast-icon"
          aria-hidden="true"
        />
        <div class="min-w-0 flex-1">
          <p v-if="notice.title" class="line-clamp-2 text-sm font-semibold leading-5 text-highlighted">
            {{ notice.title }}
          </p>
          <p v-if="notice.description" class="mt-0.5 line-clamp-2 text-xs leading-5 text-muted">
            {{ notice.description }}
          </p>
        </div>
        <UButton
          v-if="notice.close !== false"
          icon="i-tabler-x"
          color="neutral"
          variant="ghost"
          size="xs"
          square
          aria-label="关闭通知"
          class="-mr-1 -mt-1 shrink-0 text-dimmed"
          @click="toast.remove(notice.id)"
        />
      </article>
    </TransitionGroup>
  </Teleport>
</template>

<style scoped>
.yueli-toast-region {
  position: fixed;
  z-index: 110;
  bottom: 1rem;
  right: 1rem;
  display: flex;
  width: min(22rem, calc(100vw - 2rem));
  flex-direction: column;
  gap: 0.5rem;
  pointer-events: none;
}

.yueli-toast-notice {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  padding: 0.75rem;
  border: 1px solid var(--ui-border);
  border-radius: 0.75rem;
  background: var(--ui-bg);
  box-shadow: 0 14px 36px rgb(15 23 42 / 0.14);
  pointer-events: auto;
}

.yueli-toast-notice[data-tone="error"] {
  border-color: color-mix(in oklab, var(--ui-error) 28%, var(--ui-border));
}

.yueli-toast-notice[data-tone="warning"] {
  border-color: color-mix(in oklab, var(--ui-warning) 32%, var(--ui-border));
}

.yueli-toast-icon {
  width: 1.25rem;
  height: 1.25rem;
  flex: none;
  color: var(--ui-text-highlighted);
}

.yueli-toast-notice[data-tone="error"] .yueli-toast-icon {
  color: var(--ui-error);
}

.yueli-toast-notice[data-tone="warning"] .yueli-toast-icon {
  color: var(--ui-warning);
}

.yueli-toast-enter-active,
.yueli-toast-leave-active {
  transition:
    opacity 180ms var(--yueli-ease-out, ease-out),
    transform 180ms var(--yueli-ease-out, ease-out);
}

.yueli-toast-enter-from,
.yueli-toast-leave-to {
  opacity: 0;
  transform: translateY(0.5rem);
}

@media (prefers-reduced-motion: reduce) {
  .yueli-toast-enter-active,
  .yueli-toast-leave-active {
    transition: none;
  }
}
</style>
