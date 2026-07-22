<script setup lang="ts">
import { computed } from "vue";
import type { ActionFeedbackStatus } from "../../feedback/action";
import ActionFeedbackButton from "../../feedback/components/ActionFeedbackButton.vue";

export interface SettingsSaveDockMessages {
  readonly region: string;
  readonly unsaved: string;
  readonly saving: string;
  readonly saved: string;
  readonly failed: string;
  readonly discard: string;
  readonly save: string;
  readonly savePending: string;
  readonly saveSuccess: string;
}

const props = withDefaults(
  defineProps<{
    dirty: boolean;
    messages: SettingsSaveDockMessages;
    status?: ActionFeedbackStatus;
    error?: string;
    disabled?: boolean;
    dockClass?: string;
  }>(),
  { status: "idle", error: "", disabled: false, dockClass: "" },
);
const emit = defineEmits<{ discard: []; save: [] }>();
const visible = computed(
  () =>
    props.dirty ||
    props.status === "pending" ||
    props.status === "success" ||
    Boolean(props.error),
);
const title = computed(() =>
  props.error
    ? props.messages.failed
    : props.status === "pending"
      ? props.messages.saving
      : props.status === "success"
        ? props.messages.saved
        : props.messages.unsaved,
);
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition-[opacity,transform] duration-200 ease-out motion-reduce:transition-none"
      leave-active-class="transition-[opacity,transform] duration-150 ease-in motion-reduce:transition-none"
      enter-from-class="translate-y-full opacity-0"
      leave-to-class="translate-y-full opacity-0"
    >
      <section
        v-if="visible"
        data-settings-save-dock
        class="fixed inset-x-0 bottom-0 z-40 border-t border-default bg-default/95 px-3 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] shadow-[0_-12px_32px_-20px_rgba(15,23,42,0.35)] backdrop-blur"
        :class="dockClass"
        :aria-label="messages.region"
        aria-live="polite"
      >
        <div
          class="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-3"
        >
          <div class="min-w-0">
            <p
              class="flex items-center gap-2 text-sm font-medium"
              :class="error ? 'text-error' : 'text-highlighted'"
            >
              <UIcon
                :name="
                  error
                    ? 'i-tabler-alert-circle'
                    : status === 'success'
                      ? 'i-tabler-circle-check'
                      : 'i-tabler-edit-circle'
                "
                class="size-5 shrink-0"
              />
              {{ title }}
            </p>
            <p v-if="error" class="mt-0.5 truncate text-xs text-error">
              {{ error }}
            </p>
          </div>
          <div class="ml-auto flex items-center gap-2">
            <UButton
              :label="messages.discard"
              color="neutral"
              variant="ghost"
              :disabled="disabled || status === 'pending'"
              @click="emit('discard')"
            />
            <ActionFeedbackButton
              :status
              :idle-label="messages.save"
              :pending-label="messages.savePending"
              :success-label="messages.saveSuccess"
              :disabled="disabled || (!dirty && status === 'idle')"
              @click="emit('save')"
            />
          </div>
        </div>
      </section>
    </Transition>
  </Teleport>
</template>
