<script setup lang="ts">
import { computed } from "vue";
const props = defineProps<{
  subject: string;
  name?: string;
  handle?: string;
  avatarUrl?: string;
  profileUrl?: string;
  loading?: boolean;
}>();
const displayName = computed(
  () =>
    props.name ||
    props.handle ||
    (props.loading ? "加载用户资料…" : "用户资料未提供"),
);
const description = computed(() =>
  [props.handle ? `@${props.handle}` : "", props.subject]
    .filter(Boolean)
    .join(" · "),
);
</script>

<template>
  <UUser
    :name="displayName"
    :description="description"
    :avatar="
      avatarUrl
        ? { src: avatarUrl, alt: displayName }
        : { icon: 'i-tabler-user' }
    "
    :to="profileUrl"
    target="_blank"
    rel="noopener noreferrer"
    title="查看用户主页"
    class="min-w-0 max-w-full"
  />
</template>
