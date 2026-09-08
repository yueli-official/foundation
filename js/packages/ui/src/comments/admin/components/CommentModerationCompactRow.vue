<script setup lang="ts">
import AdminRowActions from "../../../admin/components/AdminRowActions.vue";
import type { CommentModerationItem } from "../types";
defineProps<{ comment: CommentModerationItem; formatDate: (value: string) => string }>();
const emit = defineEmits<{ approve: [id: string] }>();
</script>

<template>
  <div class="flex min-w-0 items-start gap-4 py-1" data-manage-comment-row data-comment-moderation-row data-comment-compact-row>
    <UAvatar :src="comment.avatarUrl" :text="(comment.authorName || '?').charAt(0)" alt="" size="sm" class="mt-0.5 shrink-0" />
    <div class="min-w-0 flex-1 space-y-2">
      <div class="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1">
        <span class="max-w-full truncate text-sm font-semibold text-highlighted">{{ comment.authorName }}</span>
        <span v-if="comment.anonymous" class="text-xs text-dimmed">访客</span>
        <time class="text-xs text-muted" :datetime="comment.createdAt">{{ formatDate(comment.createdAt) }}</time>
        <UBadge v-if="comment.status" :color="comment.status.color" variant="soft" class="h-[1.125rem] gap-1 rounded-full px-1.5 py-0 text-[11px] leading-4" data-comment-status>
          <span class="size-1 shrink-0 rounded-full bg-current" aria-hidden="true" />{{ comment.status.label }}
        </UBadge>
      </div>
      <div v-if="comment.reply" class="max-w-prose border-l border-default pl-2 text-xs leading-5 text-muted" data-comment-reply-context>
        <template v-if="comment.replyTo">
          <span>回复 @{{ comment.replyTo.authorName }}</span>
          <p class="truncate" :title="comment.replyTo.content">{{ comment.replyTo.content }}</p>
        </template>
        <span v-else>回复（原评论已不可用）</span>
      </div>
      <p class="max-w-prose whitespace-pre-wrap break-words text-sm leading-6 text-default [overflow-wrap:anywhere]" data-comment-body>{{ comment.content }}</p>
      <div class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted" data-comment-actions>
        <NuxtLink v-if="comment.source.to" :to="comment.source.to" class="inline-flex min-w-0 max-w-full items-center gap-1 hover:text-primary">
          <UIcon :name="comment.source.icon" class="size-3.5 shrink-0" /><span class="truncate">{{ comment.source.label }}</span>
        </NuxtLink>
        <span v-else class="truncate">{{ comment.source.label }}</span>
        <UButton v-if="comment.approve" label="通过" icon="i-tabler-check" size="xs" color="success" variant="ghost" class="min-h-8" :loading="comment.approving" @click="emit('approve', comment.id)" />
        <AdminRowActions v-if="comment.actions?.length" :items="comment.actions" :label="`评论操作：${comment.authorName}`" presentation="overflow" overflow-icon="i-tabler-dots" class="[--yueli-row-action-size:2rem]" />
        <details v-if="comment.authorEmail" class="basis-full" data-comment-user-details>
          <summary class="w-fit cursor-pointer text-dimmed hover:text-primary">用户详情</summary>
          <p class="break-all py-1">邮箱：{{ comment.authorEmail }}</p>
        </details>
      </div>
    </div>
    <NuxtLink v-if="comment.source.thumbnailUrl && comment.source.to" :to="comment.source.to" :aria-label="`查看图片：${comment.source.label}`" class="hidden shrink-0 self-center overflow-hidden rounded-md sm:block" data-comment-thumbnail>
      <img :src="comment.source.thumbnailUrl" alt="" width="80" height="60" loading="lazy" class="h-[60px] w-20 object-cover" />
    </NuxtLink>
  </div>
</template>
