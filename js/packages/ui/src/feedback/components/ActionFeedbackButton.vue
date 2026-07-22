<script setup lang="ts">
import { computed } from "vue";
import type { MessageResolver } from "../../messages";
import type { ActionFeedbackStatus } from "../action";

type ButtonColor =
  | "primary"
  | "secondary"
  | "success"
  | "info"
  | "warning"
  | "error"
  | "neutral";
type ButtonVariant = "solid" | "outline" | "soft" | "subtle" | "ghost" | "link";

defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    status?: ActionFeedbackStatus;
    idleLabel?: string;
    pendingLabel?: string;
    successLabel?: string;
    errorLabel?: string;
    resolveMessage?: MessageResolver;
    idleIcon?: string;
    pendingIcon?: string;
    successIcon?: string;
    errorIcon?: string;
    color?: ButtonColor;
    successColor?: ButtonColor;
    errorColor?: ButtonColor;
    variant?: ButtonVariant;
    successVariant?: ButtonVariant;
    errorVariant?: ButtonVariant;
  }>(),
  {
    status: "idle",
    idleIcon: "i-tabler-device-floppy",
    pendingIcon: "i-tabler-loader-2",
    successIcon: "i-tabler-check",
    errorIcon: "i-tabler-alert-circle",
    color: "primary",
    successColor: "success",
    errorColor: "error",
    variant: "solid",
    successVariant: "solid",
    errorVariant: "soft",
  },
);

const emit = defineEmits<{ click: [event: MouseEvent] }>();

const state = computed(() => {
  if (props.status === "pending")
    return {
      key: "foundation.feedback.action.pending",
      override: props.pendingLabel,
      icon: props.pendingIcon,
    };
  if (props.status === "success")
    return {
      key: "foundation.feedback.action.success",
      override: props.successLabel,
      icon: props.successIcon,
    };
  if (props.status === "error")
    return {
      key: "foundation.feedback.action.error",
      override: props.errorLabel,
      icon: props.errorIcon,
    };
  return {
    key: "foundation.feedback.action.idle",
    override: props.idleLabel,
    icon: props.idleIcon,
  };
});

const label = computed(
  () =>
    state.value.override ??
    props.resolveMessage?.({ key: state.value.key }) ??
    state.value.key,
);
const color = computed(() =>
  props.status === "success"
    ? props.successColor
    : props.status === "error"
      ? props.errorColor
      : props.color,
);
const variant = computed(() =>
  props.status === "success"
    ? props.successVariant
    : props.status === "error"
      ? props.errorVariant
      : props.variant,
);
</script>

<template>
  <UButton
    v-bind="$attrs"
    :label="label"
    :icon="state.icon"
    :color="color"
    :variant="variant"
    :loading="status === 'pending'"
    :aria-live="status === 'idle' ? undefined : 'polite'"
    @click="emit('click', $event)"
  />
</template>
