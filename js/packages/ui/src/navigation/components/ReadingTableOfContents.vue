<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, useId } from "vue";
import type { ReadingTableOfContentsItem } from "../table-of-contents.types";

const props = withDefaults(
  defineProps<{
    items: readonly ReadingTableOfContentsItem[];
    title?: string;
    showTitle?: boolean;
    minLevel?: number;
    maxLevel?: number;
    scrollOffset?: number;
  }>(),
  {
    title: "目录",
    showTitle: true,
    minLevel: 1,
    maxLevel: 4,
    scrollOffset: 120,
  },
);

const headingListId = `y-reading-toc-${useId().replaceAll(":", "")}`;
const headings = computed(() =>
  props.items.filter(
    (heading) =>
      heading.level >= props.minLevel && heading.level <= props.maxLevel,
  ),
);
const baseLevel = computed(() =>
  headings.value.length
    ? Math.min(...headings.value.map((heading) => heading.level))
    : props.minLevel,
);
const activeId = ref("");

function depth(level: number) {
  return Math.max(0, level - baseLevel.value);
}

function href(id: string) {
  return `#${encodeURIComponent(id)}`;
}

let frame = 0;
function updateActiveHeading() {
  if (frame) return;
  frame = requestAnimationFrame(() => {
    frame = 0;
    let current = "";
    for (const heading of headings.value) {
      const element = document.getElementById(heading.id);
      if (!element) continue;
      if (element.getBoundingClientRect().top <= props.scrollOffset) {
        current = heading.id;
        continue;
      }
      break;
    }
    activeId.value = current || headings.value[0]?.id || "";
  });
}

function navigate(event: MouseEvent, id: string) {
  const element = document.getElementById(id);
  if (!element) return;
  event.preventDefault();
  element.scrollIntoView({ behavior: "smooth", block: "start" });
  activeId.value = id;
  history.replaceState(history.state, "", href(id));
}

onMounted(() => {
  updateActiveHeading();
  window.addEventListener("scroll", updateActiveHeading, { passive: true });
  window.addEventListener("resize", updateActiveHeading, { passive: true });
});

onBeforeUnmount(() => {
  window.removeEventListener("scroll", updateActiveHeading);
  window.removeEventListener("resize", updateActiveHeading);
  if (frame) cancelAnimationFrame(frame);
});
</script>

<template>
  <nav v-if="headings.length" data-y-reading-toc :aria-label="title">
    <div v-if="showTitle" class="mb-2 flex items-center gap-2 pl-3">
      <UIcon
        name="i-tabler-list-tree"
        aria-hidden="true"
        class="size-4 text-primary"
      />
      <p class="text-sm font-semibold text-highlighted">
        {{ title }}
      </p>
    </div>
    <ul
      :id="headingListId"
      class="relative max-h-[calc(100dvh-8rem)] min-w-0 space-y-0 overflow-x-hidden overflow-y-auto border-l border-default pl-2 pr-1 [scrollbar-color:var(--ui-border-accented)_transparent] [scrollbar-width:thin] [&::-webkit-scrollbar]:w-1.5 [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-border-accented [&::-webkit-scrollbar-track]:bg-transparent"
    >
      <li v-for="heading in headings" :key="heading.id">
        <a
          :href="href(heading.id)"
          :title="heading.text"
          :data-toc-depth="depth(heading.level)"
          :aria-current="activeId === heading.id ? 'location' : undefined"
          class="relative block min-h-7 w-full min-w-0 truncate overflow-hidden py-1 pr-2 text-left leading-5 outline-none transition-colors before:absolute before:-left-2 before:top-1/2 before:h-4 before:w-px before:-translate-y-1/2 before:bg-transparent before:transition-colors focus-visible:ring-2 focus-visible:ring-primary"
          :class="[
            depth(heading.level) === 0
              ? 'text-sm font-medium'
              : depth(heading.level) === 1
                ? 'text-[13px]'
                : 'text-xs',
            activeId === heading.id
              ? 'font-medium text-primary before:bg-primary'
              : 'text-muted before:bg-transparent hover:text-highlighted',
          ]"
          :style="{ paddingLeft: `${depth(heading.level) * 10 + 6}px` }"
          @click="navigate($event, heading.id)"
        >
          {{ heading.text }}
        </a>
      </li>
    </ul>
  </nav>
</template>
