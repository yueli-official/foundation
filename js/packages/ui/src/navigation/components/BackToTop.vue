<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, useId } from "vue";
import type { MessageResolver } from "../../messages";

const BUTTON_SIZE = 36;
const MIN_SIDE_GUTTER = BUTTON_SIZE + 8;

const props = withDefaults(
  defineProps<{
    /** Caller-owned focus destination after returning to the top. */
    targetId?: string;
    /** Optional element ID for an independently scrolling container. */
    scrollContainerId?: string;
    /** Number of viewport heights scrolled before the control appears. */
    threshold?: number;
    /** Elements whose visible height/side gutter must be avoided. */
    avoidSelector?: string;
    /** Open overlays that temporarily suppress the floating control. */
    overlaySelector?: string;
    label?: string;
    resolveMessage?: MessageResolver;
  }>(),
  {
    targetId: "main-content",
    scrollContainerId: "",
    threshold: 1.5,
    avoidSelector: "[data-y-dock], [data-back-to-top-avoid]",
    overlaySelector:
      '[role="dialog"][data-state="open"], [role="alertdialog"][data-state="open"]',
  },
);

const controlId = `y-back-to-top-${useId().replaceAll(":", "")}`;
const scrolled = ref(false);
const overlayOpen = ref(false);
const dockOffset = ref(0);
const floatingLeft = ref<number>();
const reducedMotion = ref(false);
const visible = computed(() => scrolled.value && !overlayOpen.value);
const accessibleLabel = computed(
  () =>
    props.label ??
    props.resolveMessage?.({ key: "foundation.navigation.backToTop" }) ??
    "foundation.navigation.backToTop",
);
const buttonStyle = computed<Record<string, string>>(() => {
  const style: Record<string, string> = {
    "--y-back-to-top-offset": `${dockOffset.value}px`,
  };
  if (floatingLeft.value !== undefined) {
    style.left = `${floatingLeft.value}px`;
    style.right = "auto";
  }
  return style;
});

let mutationObserver: MutationObserver | undefined;
let resizeObserver: ResizeObserver | undefined;
let motionQuery: MediaQueryList | undefined;
let observedAvoidance: Element[] = [];
let scrollContainer: HTMLElement | undefined;

function isRendered(element: Element) {
  const html = element as HTMLElement;
  return (
    html.getClientRects().length > 0 &&
    html.getAttribute("aria-hidden") !== "true"
  );
}

function avoidanceOffset(element: Element) {
  const rect = element.getBoundingClientRect();
  const top = Number.isFinite(rect.top)
    ? rect.top
    : window.innerHeight - Math.max(0, rect.height);
  const bottom = Number.isFinite(rect.bottom) ? rect.bottom : top + rect.height;
  if (bottom <= 0 || top >= window.innerHeight) return 0;
  return Math.max(0, window.innerHeight - top);
}

function updateAvoidance() {
  dockOffset.value = observedAvoidance.reduce(
    (height, element) => Math.max(height, avoidanceOffset(element)),
    0,
  );

  const sideBoundaries = observedAvoidance.filter((element) =>
    element.hasAttribute("data-back-to-top-avoid"),
  );
  const boundaryRight = sideBoundaries.length
    ? Math.max(
        ...sideBoundaries.map(
          (element) => element.getBoundingClientRect().right,
        ),
      )
    : undefined;
  const gutter =
    boundaryRight === undefined ? undefined : window.innerWidth - boundaryRight;
  floatingLeft.value =
    gutter !== undefined && gutter >= MIN_SIDE_GUTTER
      ? boundaryRight! + (gutter - BUTTON_SIZE) / 2
      : undefined;
}

function syncEnvironment() {
  overlayOpen.value = Array.from(
    document.querySelectorAll(props.overlaySelector),
  ).some(isRendered);

  const nextAvoidance = Array.from(
    document.querySelectorAll(props.avoidSelector),
  ).filter(isRendered);
  if (
    nextAvoidance.length !== observedAvoidance.length ||
    nextAvoidance.some((element, index) => element !== observedAvoidance[index])
  ) {
    resizeObserver?.disconnect();
    observedAvoidance = nextAvoidance;
    if (typeof ResizeObserver !== "undefined") {
      resizeObserver = new ResizeObserver(updateAvoidance);
      observedAvoidance.forEach((element) => resizeObserver?.observe(element));
    }
  }
  updateAvoidance();
}

function currentScrollTop() {
  return scrollContainer?.scrollTop ?? window.scrollY;
}

function viewportHeight() {
  return scrollContainer?.clientHeight ?? window.innerHeight;
}

function scrollEventTarget(): HTMLElement | Window {
  return scrollContainer ?? window;
}

function updateScrollState() {
  scrolled.value = currentScrollTop() >= viewportHeight() * props.threshold;
  updateAvoidance();
}

function focusTarget(attempt = 0) {
  if (currentScrollTop() > 2 && attempt < 30 && !reducedMotion.value) {
    requestAnimationFrame(() => focusTarget(attempt + 1));
    return;
  }
  document.getElementById(props.targetId)?.focus({ preventScroll: true });
}

function returnToTop() {
  const options: ScrollToOptions = {
    top: 0,
    behavior: reducedMotion.value ? "auto" : "smooth",
  };
  if (scrollContainer) scrollContainer.scrollTo(options);
  else window.scrollTo(options);
  requestAnimationFrame(() => focusTarget());
}

function onMotionChange(event: MediaQueryListEvent) {
  reducedMotion.value = event.matches;
}

onMounted(() => {
  if (!Number.isFinite(props.threshold) || props.threshold < 0) {
    throw new RangeError("threshold must be a finite non-negative number");
  }
  scrollContainer = props.scrollContainerId
    ? (document.getElementById(props.scrollContainerId) ?? undefined)
    : undefined;
  motionQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
  reducedMotion.value = motionQuery.matches;
  motionQuery.addEventListener("change", onMotionChange);
  scrollEventTarget().addEventListener("scroll", updateScrollState, {
    passive: true,
  });
  window.addEventListener("resize", updateScrollState, { passive: true });
  mutationObserver = new MutationObserver(syncEnvironment);
  mutationObserver.observe(document.body, {
    subtree: true,
    childList: true,
    attributes: true,
    attributeFilter: ["data-state", "aria-hidden"],
  });
  updateScrollState();
  syncEnvironment();
});

onBeforeUnmount(() => {
  scrollEventTarget().removeEventListener("scroll", updateScrollState);
  window.removeEventListener("resize", updateScrollState);
  motionQuery?.removeEventListener("change", onMotionChange);
  mutationObserver?.disconnect();
  resizeObserver?.disconnect();
});
</script>

<template>
  <Transition
    enter-active-class="transition-[opacity,transform] duration-150 ease-out motion-reduce:transition-none"
    leave-active-class="transition-[opacity,transform] duration-150 ease-in motion-reduce:transition-none"
    enter-from-class="translate-y-2 opacity-0"
    leave-to-class="translate-y-2 opacity-0"
  >
    <button
      v-if="visible"
      :id="controlId"
      type="button"
      :aria-label="accessibleLabel"
      :style="buttonStyle"
      data-y-back-to-top
      class="fixed right-[max(1rem,env(safe-area-inset-right,0px))] bottom-[calc(var(--y-back-to-top-offset)+1rem+env(safe-area-inset-bottom,0px))] z-40 inline-grid size-9 place-items-center rounded-lg border border-default bg-default p-0 text-toned shadow-sm outline-none transition-colors hover:bg-elevated hover:text-highlighted focus-visible:ring-2 focus-visible:ring-primary motion-reduce:transition-none"
      @click="returnToTop"
    >
      <UIcon
        name="i-tabler-arrow-up"
        aria-hidden="true"
        class="block size-4 shrink-0"
      />
    </button>
  </Transition>
</template>
