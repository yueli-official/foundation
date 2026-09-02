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

const props = withDefaults(
  defineProps<{
    title: string;
    description?: string;
    messages: DashboardMessages;
    appearance?: "framed" | "commercial";
    pendingTitle?: string;
    pendingDescription?: string;
    recentTitle?: string;
    recentDescription?: string;
    healthTitle?: string;
    healthDescription?: string;
    actionsTitle?: string;
    actionsDescription?: string;
  }>(),
  { appearance: "framed" },
);
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
  <div class="space-y-5" :data-dashboard-appearance="appearance">
    <PageHeader :title :description size="compact">
      <template v-if="slots.actions" #actions><slot name="actions" /></template>
    </PageHeader>

    <section v-if="slots.metrics" :aria-labelledby="`${id}-metrics`">
      <h2 :id="`${id}-metrics`" class="sr-only">{{ messages.metrics }}</h2>
      <slot name="metrics" />
    </section>

    <div class="grid items-start gap-4 lg:grid-cols-3">
      <div
        class="grid min-w-0 content-start gap-4 lg:col-span-2"
        data-dashboard-column="primary"
      >
        <UCard
          v-if="slots.pending"
          as="section"
          :variant="appearance === 'commercial' ? 'soft' : 'outline'"
          data-dashboard-panel
          :class="appearance === 'commercial' ? 'bg-elevated divide-y-0' : ''"
          :ui="
            appearance === 'commercial'
              ? {
                  header: 'px-5 pb-2 pt-5 sm:px-5',
                  body: 'px-5 pb-5 pt-2 sm:px-5 sm:pb-5 sm:pt-2',
                }
              : undefined
          "
          :aria-labelledby="`${id}-pending`"
        >
          <template #header>
            <div data-dashboard-panel-header class="flex items-start gap-3">
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
            </div>
          </template>
          <slot name="pending" />
        </UCard>

        <UCard
          v-if="slots.recent"
          as="section"
          :variant="appearance === 'commercial' ? 'soft' : 'outline'"
          data-dashboard-panel
          :class="appearance === 'commercial' ? 'bg-elevated divide-y-0' : ''"
          :ui="
            appearance === 'commercial'
              ? {
                  header: 'px-5 pb-3 pt-5 sm:px-5',
                  body: 'p-0 sm:p-0',
                }
              : { body: 'p-0 sm:p-0' }
          "
          :aria-labelledby="`${id}-recent`"
        >
          <template #header>
            <div data-dashboard-panel-header class="flex items-start gap-3">
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
            </div>
          </template>
          <slot name="recent" />
        </UCard>
      </div>

      <div
        class="grid min-w-0 content-start gap-4"
        data-dashboard-column="secondary"
      >
        <UCard
          v-if="slots.health"
          as="section"
          :variant="appearance === 'commercial' ? 'soft' : 'outline'"
          data-dashboard-panel
          :class="appearance === 'commercial' ? 'bg-elevated divide-y-0' : ''"
          :ui="
            appearance === 'commercial'
              ? {
                  header: 'p-5 pb-3 sm:p-5 sm:pb-3',
                  body: 'px-5 pb-5 pt-0 sm:px-5 sm:pb-5 sm:pt-0',
                }
              : undefined
          "
          :aria-labelledby="`${id}-health`"
        >
          <template #header>
            <div data-dashboard-panel-header class="flex items-start gap-3">
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
          </template>
          <slot name="health" />
        </UCard>

        <UCard
          v-if="slots.quickActions"
          as="section"
          :variant="appearance === 'commercial' ? 'soft' : 'outline'"
          data-dashboard-panel
          :class="appearance === 'commercial' ? 'bg-elevated divide-y-0' : ''"
          :ui="
            appearance === 'commercial'
              ? {
                  header: 'p-5 pb-3 sm:p-5 sm:pb-3',
                  body: 'px-5 pb-5 pt-0 sm:px-5 sm:pb-5 sm:pt-0',
                }
              : undefined
          "
          :aria-labelledby="`${id}-actions`"
        >
          <template #header>
            <div data-dashboard-panel-header class="flex items-start gap-3">
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
          </template>
          <slot name="quickActions" />
        </UCard>
      </div>
    </div>
  </div>
</template>
