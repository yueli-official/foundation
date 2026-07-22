import { nextTick } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useActionFeedback } from "../src/feedback/action";

afterEach(() => {
  vi.useRealTimers();
});

describe("useActionFeedback", () => {
  it("resets a terminal state after the configured duration", async () => {
    vi.useFakeTimers();
    const feedback = useActionFeedback({ resetMs: 500 });

    const token = feedback.begin();
    expect(feedback.status.value).toBe("pending");
    expect(feedback.success(token)).toBe(true);
    expect(feedback.status.value).toBe("success");

    await vi.advanceTimersByTimeAsync(499);
    expect(feedback.status.value).toBe("success");
    await vi.advanceTimersByTimeAsync(1);
    expect(feedback.status.value).toBe("idle");
  });

  it("prevents an older async action from replacing the newest state", async () => {
    const feedback = useActionFeedback({ resetMs: 0 });
    let resolveFirst!: () => void;
    let resolveSecond!: () => void;
    const first = feedback.run(
      () => new Promise<void>((resolve) => (resolveFirst = resolve)),
    );
    const second = feedback.run(
      () => new Promise<void>((resolve) => (resolveSecond = resolve)),
    );

    resolveFirst();
    await first;
    await nextTick();
    expect(feedback.status.value).toBe("pending");

    resolveSecond();
    await second;
    expect(feedback.status.value).toBe("success");
  });

  it("invalidates an in-flight token on reset", () => {
    const feedback = useActionFeedback({ resetMs: 0 });
    const token = feedback.begin();
    feedback.reset();

    expect(feedback.success(token)).toBe(false);
    expect(feedback.status.value).toBe("idle");
  });

  it("supports terminal-only feedback such as copy confirmation", () => {
    const feedback = useActionFeedback({ resetMs: 0 });
    expect(feedback.success()).toBe(true);
    expect(feedback.status.value).toBe("success");
  });

  it("rejects invalid timing configuration", () => {
    expect(() => useActionFeedback({ resetMs: -1 })).toThrow(RangeError);
  });
});
