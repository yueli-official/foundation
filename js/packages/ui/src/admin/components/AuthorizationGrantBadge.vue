<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{ role: string; source?: string }>();
const sources: Record<string, string> = {
  application: "申请批准",
  invitation: "邀请加入",
  direct: "直接授予",
  automatic: "自动授权",
  group: "用户组授权",
  service_provisioning: "服务配置",
  import_sync: "导入同步",
  bootstrap: "系统初始化",
  recovery: "管理员恢复",
  initial_claim: "首次管理员认领",
};
const sourceLabel = computed(() => props.source ? sources[props.source] || "其他授权方式" : "来源未记录");
</script>

<template>
  <UTooltip :text="`授权来源：${sourceLabel}`">
    <UBadge :label="role" color="neutral" variant="soft" size="sm" tabindex="0" :aria-label="`${role}，授权来源：${sourceLabel}`" data-authorization-grant-badge />
  </UTooltip>
</template>
