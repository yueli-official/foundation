<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from "vue";
import type { PublicCommentMessages, PublicCommentViewer } from "../types";

const props = withDefaults(
  defineProps<{
    viewer?: PublicCommentViewer | null;
    messages: PublicCommentMessages;
    allowAnonymous?: boolean;
    compact?: boolean;
    busy?: boolean;
    error?: string;
    result?: "published" | "pending" | "";
    maxLength?: number;
    autofocus?: boolean;
  }>(),
  {
    viewer: null,
    allowAnonymous: false,
    compact: false,
    busy: false,
    error: "",
    result: "",
    maxLength: 5000,
    autofocus: false,
  },
);
const emit = defineEmits<{ submit: []; login: [] }>();
const content = defineModel<string>("content", { default: "" });
const authorName = defineModel<string>("authorName", { default: "" });
const authorEmail = defineModel<string>("authorEmail", { default: "" });
const textarea = ref<HTMLTextAreaElement | null>(null);
const fieldID = useId();
const authenticated = computed(() => Boolean(props.viewer?.authenticated));
const viewerInitial = computed(() =>
  (props.viewer?.name || "?").charAt(0).toUpperCase(),
);

watch(
  () => props.autofocus,
  (enabled) => {
    if (enabled) void nextTick(() => textarea.value?.focus());
  },
  { immediate: true },
);
</script>

<template>
  <div
    class="overflow-hidden rounded-xl border border-default bg-default transition-colors focus-within:border-primary"
    :class="compact ? 'rounded-lg' : ''"
    data-public-comment-composer
  >
    <label :for="fieldID" class="sr-only">
      {{ compact ? messages.writeReply : messages.writeComment }}
    </label>
    <textarea
      :id="fieldID"
      ref="textarea"
      v-model="content"
      dir="auto"
      :rows="compact ? 2 : 4"
      :maxlength="maxLength"
      :placeholder="compact ? messages.writeReply : messages.writeComment"
      class="block min-h-24 w-full resize-y border-0 bg-transparent px-3 py-3 text-sm leading-6 text-highlighted outline-none placeholder:text-dimmed disabled:cursor-not-allowed disabled:opacity-60"
      :class="compact ? 'min-h-20' : 'sm:min-h-28'"
      :disabled="busy"
      @keydown.meta.enter.prevent="emit('submit')"
      @keydown.ctrl.enter.prevent="emit('submit')"
    />

    <div
      v-if="!authenticated && allowAnonymous"
      class="grid gap-2 border-t border-default bg-elevated/25 p-3 sm:grid-cols-2"
    >
      <UInput
        v-model="authorName"
        :placeholder="messages.authorName"
        size="sm"
        maxlength="40"
      />
      <UInput
        v-model="authorEmail"
        :placeholder="messages.authorEmail"
        size="sm"
        type="email"
      />
    </div>

    <div
      v-if="error || result"
      class="border-t border-default px-3 py-2 text-sm"
      role="status"
      aria-live="polite"
    >
      <span v-if="error" class="inline-flex items-center gap-1.5 text-error">
        <UIcon name="i-tabler-alert-circle" class="size-4" />{{ error }}
      </span>
      <span
        v-else
        class="inline-flex items-center gap-1.5"
        :class="result === 'pending' ? 'text-info' : 'text-success'"
      >
        <UIcon
          :name="
            result === 'pending' ? 'i-tabler-clock' : 'i-tabler-circle-check'
          "
          class="size-4"
        />
        {{ result === "pending" ? messages.pending : messages.submitted }}
      </span>
    </div>

    <footer
      class="flex min-h-11 flex-wrap items-center gap-3 border-t border-default bg-elevated/35 px-3 py-2"
    >
      <div class="min-w-0 flex-1 text-xs text-muted">
        <span
          v-if="authenticated"
          class="inline-flex min-w-0 items-center gap-2"
        >
          <UAvatar
            :src="viewer?.avatarUrl"
            :text="viewerInitial"
            :alt="viewer?.name || ''"
            size="2xs"
          />
          <span class="truncate">{{ viewer?.name }}</span>
        </span>
        <span v-else>
          <span v-if="allowAnonymous" class="me-1">
            {{ messages.anonymousHint }} ·
          </span>
          <button
            type="button"
            class="font-medium text-primary hover:underline"
            @click="emit('login')"
          >
            {{ messages.login }}
          </button>
        </span>
      </div>
      <span class="text-xs tabular-nums text-dimmed">
        {{ content.length }}/{{ maxLength }}
      </span>
      <UButton
        :label="compact ? messages.submitReply : messages.submit"
        icon="i-tabler-send"
        size="sm"
        :loading="busy"
        :disabled="!content.trim() || (!authenticated && !allowAnonymous)"
        @click="emit('submit')"
      />
    </footer>
  </div>
</template>
