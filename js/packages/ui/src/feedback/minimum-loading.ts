import {
  getCurrentScope,
  onScopeDispose,
  readonly,
  ref,
  watch,
  type Ref,
} from "vue";

export interface MinimumLoadingOptions {
  /** Minimum time the indicator remains visible after it is shown. */
  readonly minimumMs?: number;
  /** Delay before showing an indicator, avoiding flashes for fast operations. */
  readonly delayMs?: number;
}

export function useMinimumLoading(
  loading: Readonly<Ref<boolean>>,
  options: MinimumLoadingOptions = {},
): Readonly<Ref<boolean>> {
  const minimumMs = options.minimumMs ?? 500;
  const delayMs = options.delayMs ?? 0;
  if (
    !Number.isFinite(minimumMs) ||
    minimumMs < 0 ||
    !Number.isFinite(delayMs) ||
    delayMs < 0
  ) {
    throw new RangeError(
      "minimumMs and delayMs must be finite non-negative numbers",
    );
  }

  const visible = ref(loading.value && delayMs === 0);
  let visibleSince = visible.value ? Date.now() : 0;
  let showTimer: ReturnType<typeof setTimeout> | undefined;
  let hideTimer: ReturnType<typeof setTimeout> | undefined;

  function clearShowTimer() {
    if (showTimer === undefined) return;
    clearTimeout(showTimer);
    showTimer = undefined;
  }

  function clearHideTimer() {
    if (hideTimer === undefined) return;
    clearTimeout(hideTimer);
    hideTimer = undefined;
  }

  function show() {
    clearShowTimer();
    clearHideTimer();
    visibleSince = Date.now();
    visible.value = true;
  }

  function hide() {
    clearShowTimer();
    if (!visible.value) return;
    const remaining = Math.max(0, minimumMs - (Date.now() - visibleSince));
    clearHideTimer();
    if (remaining === 0) {
      visible.value = false;
      return;
    }
    hideTimer = setTimeout(() => {
      visible.value = false;
      hideTimer = undefined;
    }, remaining);
  }

  const stop = watch(
    loading,
    (next) => {
      if (!next) {
        hide();
        return;
      }
      clearHideTimer();
      if (visible.value) return;
      if (delayMs === 0) show();
      else {
        clearShowTimer();
        showTimer = setTimeout(() => {
          if (loading.value) show();
        }, delayMs);
      }
    },
    { immediate: loading.value && delayMs > 0 },
  );

  function dispose() {
    stop();
    clearShowTimer();
    clearHideTimer();
  }

  if (getCurrentScope()) onScopeDispose(dispose);
  return readonly(visible);
}
