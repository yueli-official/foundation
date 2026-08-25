<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import type { ContentShareMessages } from "../content-share.types";

const props = defineProps<{
  title: string;
  messages: ContentShareMessages;
  url?: string;
}>();

const resolvedUrl = ref("");
const canUseSystemShare = ref(false);
const copyState = ref<"idle" | "copied" | "failed">("idle");
let resetTimer: ReturnType<typeof setTimeout> | undefined;

function syncUrl() {
  resolvedUrl.value =
    props.url || (import.meta.client ? window.location.href : "");
}

watch(() => props.url, syncUrl);
onMounted(() => {
  syncUrl();
  canUseSystemShare.value = typeof navigator.share === "function";
});
onBeforeUnmount(() => {
  if (resetTimer) clearTimeout(resetTimer);
});

const targets = computed(() => {
  const url = encodeURIComponent(resolvedUrl.value);
  const title = encodeURIComponent(props.title);
  return [
    {
      key: "weibo",
      label: props.messages.weibo,
      icon: "i-tabler-brand-weibo",
      href: `https://service.weibo.com/share/share.php?url=${url}&title=${title}`,
    },
    {
      key: "x",
      label: props.messages.x,
      icon: "i-tabler-brand-x",
      href: `https://twitter.com/intent/tweet?url=${url}&text=${title}`,
    },
  ];
});

function scheduleReset() {
  if (resetTimer) clearTimeout(resetTimer);
  resetTimer = setTimeout(() => {
    copyState.value = "idle";
  }, 1800);
}

function fallbackCopy(text: string) {
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.append(textarea);
  textarea.select();
  const copied = document.execCommand("copy");
  textarea.remove();
  if (!copied) throw new Error("clipboard unavailable");
}

async function copyLink() {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(resolvedUrl.value);
    } else {
      fallbackCopy(resolvedUrl.value);
    }
    copyState.value = "copied";
  } catch {
    copyState.value = "failed";
  }
  scheduleReset();
}

async function systemShare() {
  try {
    await navigator.share({ title: props.title, url: resolvedUrl.value });
  } catch {
    // Dismissing the operating-system share sheet is not an error state.
  }
}

const copyPresentation = computed(() => {
  if (copyState.value === "copied") {
    return {
      label: props.messages.copied,
      icon: "i-tabler-check",
      color: "primary" as const,
    };
  }
  if (copyState.value === "failed") {
    return {
      label: props.messages.copyFailed,
      icon: "i-tabler-alert-circle",
      color: "error" as const,
    };
  }
  return {
    label: props.messages.copy,
    icon: "i-tabler-link",
    color: "neutral" as const,
  };
});
</script>

<template>
  <div v-if="resolvedUrl" data-y-content-share class="flex items-center gap-1">
    <UButton
      v-for="target in targets"
      :key="target.key"
      :icon="target.icon"
      :to="target.href"
      target="_blank"
      rel="noopener noreferrer"
      color="neutral"
      variant="ghost"
      size="sm"
      square
      :aria-label="target.label"
      :title="target.label"
    />
    <UButton
      v-if="canUseSystemShare"
      icon="i-tabler-share-2"
      color="neutral"
      variant="ghost"
      size="sm"
      square
      :aria-label="messages.system"
      :title="messages.system"
      @click="systemShare"
    />
    <UButton
      data-share-copy
      :data-copy-state="copyState"
      :icon="copyPresentation.icon"
      :color="copyPresentation.color"
      :label="copyPresentation.label"
      variant="ghost"
      size="sm"
      :aria-label="copyPresentation.label"
      @click="copyLink"
    >
      <template #trailing>
        <span class="sr-only" aria-live="polite">{{
          copyPresentation.label
        }}</span>
      </template>
    </UButton>
  </div>
</template>
