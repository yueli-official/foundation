import { computed, getCurrentScope, onScopeDispose, readonly, ref } from "vue";

export type ActionFeedbackStatus = "idle" | "pending" | "success" | "error";

declare const actionFeedbackTokenBrand: unique symbol;
export type ActionFeedbackToken = number & {
  readonly [actionFeedbackTokenBrand]: true;
};

export interface ActionFeedbackOptions {
  /** Duration of a terminal state. Zero keeps it until the next action/reset. */
  readonly resetMs?: number;
}

/**
 * Owns one latest-wins action lifecycle. A stale async operation cannot replace
 * the visible state produced by a newer operation.
 */
export function useActionFeedback(options: ActionFeedbackOptions = {}) {
  const resetMs = options.resetMs ?? 1_600;
  if (!Number.isFinite(resetMs) || resetMs < 0) {
    throw new RangeError("resetMs must be a finite non-negative number");
  }

  const status = ref<ActionFeedbackStatus>("idle");
  let sequence = 0;
  let activeToken: ActionFeedbackToken | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;

  function clearTimer() {
    if (timer === undefined) return;
    clearTimeout(timer);
    timer = undefined;
  }

  function scheduleReset(token: ActionFeedbackToken) {
    if (resetMs === 0) return;
    timer = setTimeout(() => {
      if (activeToken === token) {
        activeToken = undefined;
        status.value = "idle";
      }
      timer = undefined;
    }, resetMs);
  }

  function begin(): ActionFeedbackToken {
    clearTimer();
    const token = ++sequence as ActionFeedbackToken;
    activeToken = token;
    status.value = "pending";
    return token;
  }

  function settle(
    next: Extract<ActionFeedbackStatus, "success" | "error">,
    token = activeToken,
  ): boolean {
    if (token === undefined || token !== activeToken) return false;
    clearTimer();
    status.value = next;
    scheduleReset(token);
    return true;
  }

  function reset() {
    clearTimer();
    sequence += 1;
    activeToken = undefined;
    status.value = "idle";
  }

  function finish(
    next: Extract<ActionFeedbackStatus, "success" | "error">,
    token?: ActionFeedbackToken,
  ) {
    if (token !== undefined) return settle(next, token);
    if (activeToken !== undefined) return settle(next, activeToken);
    clearTimer();
    const terminalToken = ++sequence as ActionFeedbackToken;
    activeToken = terminalToken;
    status.value = next;
    scheduleReset(terminalToken);
    return true;
  }

  async function run<T>(action: () => T | Promise<T>): Promise<T> {
    const token = begin();
    try {
      const result = await action();
      settle("success", token);
      return result;
    } catch (error) {
      settle("error", token);
      throw error;
    }
  }

  if (getCurrentScope()) onScopeDispose(reset);

  return {
    status: readonly(status),
    isPending: computed(() => status.value === "pending"),
    begin,
    pending: begin,
    success: (token?: ActionFeedbackToken) => finish("success", token),
    error: (token?: ActionFeedbackToken) => finish("error", token),
    run,
    reset,
  } as const;
}
