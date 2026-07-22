<script setup lang="ts">
import { computed, useId, useSlots } from "vue";
import PageHeader from "./PageHeader.vue";

export interface DashboardSectionMessages {
  readonly title: string;
  readonly description: string;
}

export interface DashboardMessages {
  readonly metrics: string;
  readonly pending: DashboardSectionMessages;
  readonly recent: DashboardSectionMessages;
  readonly health: DashboardSectionMessages;
  readonly quickActions: DashboardSectionMessages;
}

const props = defineProps<{
  title: string;
  description?: string;
  messages: DashboardMessages;
  pendingTitle?: string;
  pendingDescription?: string;
  recentTitle?: string;
  recentDescription?: string;
  healthTitle?: string;
  healthDescription?: string;
  actionsTitle?: string;
  actionsDescription?: string;
}>();
const slots = useSlots();
const id = useId().replaceAll(":", "");
const copy = computed(() => ({
  pending: {
    title: props.pendingTitle || props.messages.pending.title,
    description: props.pendingDescription || props.messages.pending.description,
  },
  recent: {
    title: props.recentTitle || props.messages.recent.title,
    description: props.recentDescription || props.messages.recent.description,
  },
  health: {
    title: props.healthTitle || props.messages.health.title,
    description: props.healthDescription || props.messages.health.description,
  },
  quickActions: {
    title: props.actionsTitle || props.messages.quickActions.title,
    description:
      props.actionsDescription || props.messages.quickActions.description,
  },
}));
</script>

<template>
  <div class="space-y-5">
    <PageHeader :title :description>
      <template v-if="slots.actions" #actions><slot name="actions" /></template>
    </PageHeader>

    <section v-if="slots.metrics" :aria-labelledby="`${id}-metrics`">
      <h2 :id="`${id}-metrics`" class="sr-only">{{ messages.metrics }}</h2>
      <slot name="metrics" />
    </section>

    <div class="grid items-stretch gap-4 xl:grid-cols-12">
      <section
        v-if="slots.pending"
        class="overflow-hidden rounded-xl border border-default bg-default shadow-sm xl:col-span-8"
        :aria-labelledby="`${id}-pending`"
      >
        <header
          class="flex items-start gap-3 border-b border-default px-5 py-4"
        >
          <span
            class="grid size-8 shrink-0 place-items-center rounded-lg bg-warning/10 text-warning"
            aria-hidden="true"
          >
            <UIcon name="i-tabler-alert-circle" class="size-4" />
          </span>
          <div class="min-w-0">
            <h2
              :id="`${id}-pending`"
              class="text-sm font-semibold text-highlighted"
            >
              {{ copy.pending.title }}
            </h2>
            <p class="mt-0.5 text-xs leading-5 text-muted">
              {{ copy.pending.description }}
            </p>
          </div>
        </header>
        <div class="p-5"><slot name="pending" /></div>
      </section>

      <section
        v-if="slots.health"
        class="rounded-xl border border-default bg-default p-5 shadow-sm xl:col-span-4"
        :aria-labelledby="`${id}-health`"
      >
        <div class="flex items-start gap-3">
          <span
            class="grid size-8 shrink-0 place-items-center rounded-lg bg-success/10 text-success"
            aria-hidden="true"
          >
            <UIcon name="i-tabler-heart-rate-monitor" class="size-4" />
          </span>
          <div class="min-w-0">
            <h2
              :id="`${id}-health`"
              class="text-sm font-semibold text-highlighted"
            >
              {{ copy.health.title }}
            </h2>
            <p class="mt-0.5 text-xs leading-5 text-muted">
              {{ copy.health.description }}
            </p>
          </div>
        </div>
        <div class="mt-5"><slot name="health" /></div>
      </section>

      <section
        v-if="slots.recent"
        class="overflow-hidden rounded-xl border border-default bg-default shadow-sm xl:col-span-8"
        :aria-labelledby="`${id}-recent`"
      >
        <header
          class="flex items-start gap-3 border-b border-default px-5 py-4"
        >
          <span
            class="grid size-8 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary"
            aria-hidden="true"
          >
            <UIcon name="i-tabler-history" class="size-4" />
          </span>
          <div class="min-w-0">
            <h2
              :id="`${id}-recent`"
              class="text-sm font-semibold text-highlighted"
            >
              {{ copy.recent.title }}
            </h2>
            <p class="mt-0.5 text-xs leading-5 text-muted">
              {{ copy.recent.description }}
            </p>
          </div>
        </header>
        <slot name="recent" />
      </section>

      <section
        v-if="slots.quickActions"
        class="rounded-xl border border-default bg-default p-5 shadow-sm xl:col-span-4"
        :aria-labelledby="`${id}-actions`"
      >
        <div class="flex items-start gap-3">
          <span
            class="grid size-8 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary"
            aria-hidden="true"
          >
            <UIcon name="i-tabler-bolt" class="size-4" />
          </span>
          <div class="min-w-0">
            <h2
              :id="`${id}-actions`"
              class="text-sm font-semibold text-highlighted"
            >
              {{ copy.quickActions.title }}
            </h2>
            <p class="mt-0.5 text-xs leading-5 text-muted">
              {{ copy.quickActions.description }}
            </p>
          </div>
        </div>
        <div class="mt-5"><slot name="quickActions" /></div>
      </section>
    </div>
  </div>
</template>
