<script setup lang="ts">
import { computed } from "vue";
import AuthorizationUser from "./AuthorizationUser.vue";
const props = defineProps<{
  user: { subject: string; name?: string; handle?: string; avatarUrl?: string; profileUrl?: string; loading?: boolean };
  role: string;
  reason?: string;
  createdAt: string;
  busy?: boolean;
}>();
const emit = defineEmits<{ review: [decision: "approve" | "reject"] }>();
const timestamp = computed(() => {
  const value = Date.parse(props.createdAt);
  return Number.isFinite(value) ? new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit",
  }).format(value) : "未记录";
});
</script>

<template>
  <article class="grid gap-3 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center sm:px-5" data-authorization-application>
    <div class="min-w-0">
      <div class="flex flex-wrap items-center gap-2">
        <AuthorizationUser v-bind="user" />
        <UBadge :label="role" color="neutral" variant="outline" size="sm" />
      </div>
      <time class="mt-2 block text-xs text-muted" :datetime="createdAt || undefined">申请时间：{{ timestamp }}</time>
      <p class="mt-2 whitespace-pre-wrap break-words text-sm text-muted">{{ reason || "未填写申请理由" }}</p>
    </div>
    <div class="flex gap-2 sm:justify-end">
      <UButton label="拒绝" color="neutral" variant="outline" :disabled="busy" @click="emit('review', 'reject')" />
      <UButton label="批准" :disabled="busy" @click="emit('review', 'approve')" />
    </div>
  </article>
</template>
