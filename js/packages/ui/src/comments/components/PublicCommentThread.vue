<script setup lang="ts">
import { computed, ref } from "vue";
import PublicCommentComposer from "./PublicCommentComposer.vue";
import type {
  PublicComment,
  PublicCommentDraft,
  PublicCommentMessages,
  PublicCommentOrder,
  PublicCommentState,
  PublicCommentSubmit,
  PublicCommentViewer,
} from "../types";

const props = withDefaults(
  defineProps<{
    comments?: readonly PublicComment[];
    total?: number;
    state?: PublicCommentState;
    order?: PublicCommentOrder;
    viewer?: PublicCommentViewer | null;
    messages: PublicCommentMessages;
    formatTime: (value: string) => string;
    submit?: PublicCommentSubmit;
    login?: () => void;
    retry?: () => void;
    allowAnonymous?: boolean;
    closed?: boolean;
    inputPosition?: "top" | "bottom";
    maxLength?: number;
  }>(),
  {
    comments: () => [],
    total: 0,
    state: "ready",
    order: "asc",
    viewer: null,
    submit: undefined,
    login: undefined,
    retry: undefined,
    allowAnonymous: false,
    closed: false,
    inputPosition: "bottom",
    maxLength: 5000,
  },
);
const emit = defineEmits<{
  "update:order": [value: PublicCommentOrder];
  submitted: [result: { pending: boolean; parentId?: string }];
}>();

const activeReplyID = ref("");
const topContent = ref("");
const replyContent = ref("");
const authorName = ref("");
const authorEmail = ref("");
const busyTarget = ref("");
const errors = ref<Record<string, string>>({});
const results = ref<Record<string, "published" | "pending" | "">>({});
const canCompose = computed(() => Boolean(props.submit) && !props.closed);

function initial(name: string) {
  return (name || "?").charAt(0).toUpperCase();
}

function toggleReply(id: string) {
  activeReplyID.value = activeReplyID.value === id ? "" : id;
  replyContent.value = "";
  errors.value[id] = "";
  results.value[id] = "";
}

function openReply(id: string) {
  if (activeReplyID.value === id) return;
  activeReplyID.value = id;
  replyContent.value = "";
  errors.value[id] = "";
  results.value[id] = "";
}

async function submitDraft(parentId = "") {
  if (!props.submit || busyTarget.value) return;
  const key = parentId || "root";
  const content = (parentId ? replyContent.value : topContent.value).trim();
  if (!content) return;
  if (
    !props.viewer?.authenticated &&
    props.allowAnonymous &&
    !authorName.value.trim()
  ) {
    errors.value[key] = props.messages.nameRequired;
    return;
  }
  busyTarget.value = key;
  errors.value[key] = "";
  results.value[key] = "";
  const draft: PublicCommentDraft = {
    content,
    ...(parentId ? { parentId } : {}),
    ...(!props.viewer?.authenticated
      ? {
          authorName: authorName.value.trim(),
          authorEmail: authorEmail.value.trim(),
        }
      : {}),
  };
  try {
    const result = await props.submit(draft);
    results.value[key] = result.pending ? "pending" : "published";
    if (parentId) {
      replyContent.value = "";
      if (!result.pending) activeReplyID.value = "";
    } else {
      topContent.value = "";
    }
    emit("submitted", {
      pending: result.pending,
      ...(parentId ? { parentId } : {}),
    });
  } catch (error) {
    errors.value[key] =
      error && typeof error === "object" && "message" in error
        ? String(error.message)
        : props.messages.submitError;
  } finally {
    busyTarget.value = "";
  }
}
</script>

<template>
  <section class="space-y-5" data-public-comment-thread>
    <header class="flex flex-wrap items-center justify-between gap-3">
      <h2
        class="font-display flex items-center gap-2 text-base font-semibold text-highlighted sm:text-lg"
      >
        <UIcon
          name="i-tabler-messages"
          class="size-5 shrink-0 text-primary"
          aria-hidden="true"
        />
        {{ messages.count(total) }}
      </h2>
      <div
        v-if="total > 1"
        class="inline-grid grid-cols-2 rounded-lg border border-default bg-elevated p-0.5"
        role="group"
        :aria-label="messages.sort"
      >
        <button
          type="button"
          class="min-h-7 rounded-md border px-3 text-xs transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary disabled:cursor-wait disabled:opacity-60"
          :class="
            order === 'asc'
              ? 'border-default bg-default font-medium text-highlighted shadow-sm'
              : 'border-transparent text-muted hover:text-default'
          "
          :aria-pressed="order === 'asc'"
          :disabled="state === 'loading'"
          @click="emit('update:order', 'asc')"
        >
          {{ messages.oldest }}
        </button>
        <button
          type="button"
          class="min-h-7 rounded-md border px-3 text-xs transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary disabled:cursor-wait disabled:opacity-60"
          :class="
            order === 'desc'
              ? 'border-default bg-default font-medium text-highlighted shadow-sm'
              : 'border-transparent text-muted hover:text-default'
          "
          :aria-pressed="order === 'desc'"
          :disabled="state === 'loading'"
          @click="emit('update:order', 'desc')"
        >
          {{ messages.newest }}
        </button>
      </div>
    </header>

    <PublicCommentComposer
      v-if="canCompose && inputPosition === 'top'"
      v-model:content="topContent"
      v-model:author-name="authorName"
      v-model:author-email="authorEmail"
      :viewer="viewer"
      :messages="messages"
      :allow-anonymous="allowAnonymous"
      :busy="busyTarget === 'root'"
      :error="errors.root"
      :result="results.root"
      :max-length="maxLength"
      @login="login?.()"
      @submit="submitDraft()"
    />

    <div
      v-if="state === 'loading'"
      class="space-y-3"
      role="status"
      :aria-label="messages.loading"
    >
      <div
        v-for="index in 2"
        :key="index"
        class="rounded-xl border border-default p-4"
      >
        <div class="flex items-center gap-3">
          <USkeleton class="size-8 rounded-full" />
          <USkeleton class="h-4 w-36" />
        </div>
        <USkeleton class="mt-4 h-4 w-4/5" />
        <USkeleton class="mt-2 h-4 w-2/3" />
      </div>
    </div>

    <UAlert
      v-else-if="state === 'error'"
      color="error"
      variant="subtle"
      icon="i-tabler-alert-circle"
      :title="messages.loadError"
      :actions="
        retry
          ? [
              {
                label: messages.retry,
                color: 'error',
                variant: 'soft',
                onClick: retry,
              },
            ]
          : []
      "
    />

    <p v-else-if="!comments.length" class="py-5 text-center text-sm text-muted">
      {{ messages.empty }}
    </p>

    <ol v-else class="space-y-4">
      <li
        v-for="comment in comments"
        :key="comment.id"
        class="overflow-hidden rounded-xl border border-default bg-default"
        data-public-comment
        :data-comment-id="comment.id"
      >
        <article class="p-4 sm:p-5">
          <header class="flex min-w-0 items-center gap-3">
            <UAvatar
              :src="comment.avatarUrl"
              :text="initial(comment.authorName)"
              :alt="comment.authorName"
              size="sm"
              class="shrink-0"
            />
            <div class="min-w-0 flex-1">
              <div
                class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-sm"
              >
                <span class="truncate font-semibold text-highlighted">
                  {{ comment.authorName }}
                </span>
                <UBadge
                  v-if="comment.isAnonymous"
                  :label="messages.anonymous"
                  color="neutral"
                  variant="subtle"
                  size="xs"
                />
              </div>
            </div>
          </header>
          <p
            dir="auto"
            class="mt-3 whitespace-pre-wrap break-words text-sm leading-6 text-default"
          >
            {{ comment.content }}
          </p>
          <footer
            class="mt-2.5 flex min-h-7 flex-wrap items-center justify-between gap-x-4 gap-y-2"
          >
            <div class="flex items-center gap-1 text-xs text-muted">
              <time :datetime="comment.createdAt">
                {{ formatTime(comment.createdAt) }}
              </time>
              <UButton
                v-if="canCompose"
                :label="
                  activeReplyID === comment.id
                    ? messages.cancelReply
                    : messages.reply
                "
                size="xs"
                color="neutral"
                variant="ghost"
                class="-my-1 min-h-7 px-1.5 text-xs font-normal text-muted hover:text-primary"
                @click="toggleReply(comment.id)"
              />
            </div>
            <span v-if="comment.replies?.length" class="text-xs text-muted">
              {{ messages.replies(comment.replies.length) }}
            </span>
          </footer>
        </article>

        <div
          v-if="comment.replies?.length || activeReplyID === comment.id"
          class="border-t border-default bg-elevated/35"
        >
          <ol
            v-if="comment.replies?.length"
            class="relative divide-y divide-default"
          >
            <li
              v-for="reply in comment.replies"
              :key="reply.id"
              class="relative flex gap-3 px-4 py-4 sm:px-5"
              :data-reply-id="reply.id"
            >
              <span
                class="absolute bottom-0 start-[1.95rem] top-0 w-px bg-default"
                aria-hidden="true"
              />
              <UAvatar
                :src="reply.avatarUrl"
                :text="initial(reply.authorName)"
                :alt="reply.authorName"
                size="2xs"
                class="relative z-10 shrink-0"
              />
              <div class="min-w-0 flex-1">
                <div
                  class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm"
                >
                  <span class="font-semibold text-highlighted">
                    {{ reply.authorName }}
                  </span>
                  <UBadge
                    v-if="reply.isAnonymous"
                    :label="messages.anonymous"
                    color="neutral"
                    variant="subtle"
                    size="xs"
                  />
                </div>
                <p
                  dir="auto"
                  class="mt-1.5 whitespace-pre-wrap break-words text-sm leading-6 text-default"
                >
                  {{ reply.content }}
                </p>
                <footer
                  class="mt-2 flex min-h-7 items-center gap-1 text-xs text-muted"
                >
                  <time :datetime="reply.createdAt">
                    {{ formatTime(reply.createdAt) }}
                  </time>
                  <UButton
                    v-if="canCompose"
                    :label="messages.reply"
                    size="xs"
                    color="neutral"
                    variant="ghost"
                    class="-my-1 min-h-7 px-1.5 text-xs font-normal text-muted hover:text-primary"
                    @click="openReply(comment.id)"
                  />
                </footer>
              </div>
            </li>
          </ol>
          <div
            v-if="activeReplyID === comment.id"
            class="border-t border-default p-3 sm:p-4"
          >
            <PublicCommentComposer
              v-model:content="replyContent"
              v-model:author-name="authorName"
              v-model:author-email="authorEmail"
              :viewer="viewer"
              :messages="messages"
              :allow-anonymous="allowAnonymous"
              :busy="busyTarget === comment.id"
              :error="errors[comment.id]"
              :result="results[comment.id]"
              :max-length="maxLength"
              compact
              autofocus
              @login="login?.()"
              @submit="submitDraft(comment.id)"
            />
          </div>
        </div>
      </li>
    </ol>

    <UAlert
      v-if="closed"
      color="neutral"
      variant="subtle"
      icon="i-tabler-lock"
      :description="messages.closed"
    />

    <PublicCommentComposer
      v-else-if="canCompose && inputPosition === 'bottom'"
      v-model:content="topContent"
      v-model:author-name="authorName"
      v-model:author-email="authorEmail"
      :viewer="viewer"
      :messages="messages"
      :allow-anonymous="allowAnonymous"
      :busy="busyTarget === 'root'"
      :error="errors.root"
      :result="results.root"
      :max-length="maxLength"
      @login="login?.()"
      @submit="submitDraft()"
    />
  </section>
</template>
