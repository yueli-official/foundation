<script setup lang="ts">
import { computed } from "vue";
import AdminRowActions from "../../../admin/components/AdminRowActions.vue";
import type { AdminRowActionItem } from "../../../admin/types";
import type { CommentModerationItem } from "../types";

const props = defineProps<{
  comment: CommentModerationItem;
  formatDate: (value: string) => string;
}>();
const emit = defineEmits<{ approve: [id: string] }>();
const menuItems = computed(() => {
  const items = props.comment.actions || [];
  const groups: (readonly AdminRowActionItem[])[] = Array.isArray(items[0])
    ? ([...items] as (readonly AdminRowActionItem[])[])
    : items.length
      ? [items as readonly AdminRowActionItem[]]
      : [];
  if (props.comment.approve)
    groups.unshift([
      {
        id: "approve",
        label: "通过审核",
        icon: "i-tabler-check",
        loading: props.comment.approving,
        onSelect: () => emit("approve", props.comment.id),
      },
    ]);
  return groups;
});
</script>

<template>
  <div
    class="comment-columns min-w-0 py-1"
    data-manage-comment-row
    data-comment-moderation-row
    data-comment-columns-row
  >
    <div class="col-span-3 flex min-w-0 items-start gap-3 lg:col-span-1">
      <UAvatar
        :src="comment.avatarUrl"
        :text="(comment.authorName || '?').charAt(0)"
        alt=""
        size="sm"
        class="mt-0.5 shrink-0"
      />
      <div class="min-w-0 flex-1">
        <div class="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1">
          <span class="truncate text-sm font-semibold text-highlighted">{{
            comment.authorName
          }}</span>
          <span v-if="comment.anonymous" class="text-xs text-dimmed">访客</span>
          <time class="text-xs text-muted" :datetime="comment.createdAt">{{
            formatDate(comment.createdAt)
          }}</time>
        </div>
        <div
          v-if="comment.reply"
          class="mt-2 border-l border-default pl-2 text-xs leading-5 text-muted"
          data-comment-reply-context
        >
          <template v-if="comment.replyTo">
            <span>回复 @{{ comment.replyTo.authorName }}</span>
            <p class="truncate" :title="comment.replyTo.content">
              {{ comment.replyTo.content }}
            </p>
          </template>
          <span v-else>回复（原评论已不可用）</span>
        </div>
        <p
          class="mt-1 whitespace-pre-wrap text-sm leading-6 text-default [overflow-wrap:anywhere]"
          data-comment-body
        >
          {{ comment.content }}
        </p>
        <details
          v-if="comment.authorEmail"
          class="mt-1 text-xs text-muted"
          data-comment-user-details
        >
          <summary class="w-fit cursor-pointer hover:text-primary">
            用户详情
          </summary>
          <p class="break-all py-1">邮箱：{{ comment.authorEmail }}</p>
        </details>
      </div>
    </div>
    <div class="min-w-0 pl-10 text-xs text-muted lg:pl-0" data-comment-source>
      <NuxtLink
        v-if="comment.source.to"
        :to="comment.source.to"
        class="flex min-w-0 items-center gap-1.5 hover:text-primary"
        :title="comment.source.label"
      >
        <UIcon :name="comment.source.icon" class="size-3.5 shrink-0" /><span
          class="truncate"
          >{{ comment.source.label }}</span
        >
      </NuxtLink>
      <span v-else class="block truncate" :title="comment.source.label">{{
        comment.source.label
      }}</span>
    </div>
    <div>
      <UBadge
        v-if="comment.status"
        :color="comment.status.color"
        variant="soft"
        class="h-[1.125rem] gap-1 rounded-full px-1.5 py-0 text-[11px] leading-4"
        data-comment-status
      >
        <span
          class="size-1 shrink-0 rounded-full bg-current"
          aria-hidden="true"
        />{{ comment.status.label }}
      </UBadge>
    </div>
    <div class="flex justify-end">
      <AdminRowActions
        v-if="menuItems.length"
        :items="menuItems"
        :label="`评论操作：${comment.authorName}`"
        presentation="overflow"
        overflow-icon="i-tabler-dots"
        class="[--yueli-row-action-size:2rem]"
      />
    </div>
  </div>
</template>
