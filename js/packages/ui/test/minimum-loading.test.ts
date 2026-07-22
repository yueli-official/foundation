import { nextTick, ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useMinimumLoading } from "../src/feedback/minimum-loading";

afterEach(() => {
  vi.useRealTimers();
});

describe("useMinimumLoading", () => {
  it("delays visibility then keeps it stable for the minimum duration", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(0);
    const loading = ref(false);
    const visible = useMinimumLoading(loading, {
      delayMs: 100,
      minimumMs: 300,
    });

    loading.value = true;
    await nextTick();
    await vi.advanceTimersByTimeAsync(99);
    expect(visible.value).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(visible.value).toBe(true);

    loading.value = false;
    await nextTick();
    await vi.advanceTimersByTimeAsync(299);
    expect(visible.value).toBe(true);
    await vi.advanceTimersByTimeAsync(1);
    expect(visible.value).toBe(false);
  });

  it("never shows an operation that finishes inside the delay", async () => {
    vi.useFakeTimers();
    const loading = ref(false);
    const visible = useMinimumLoading(loading, {
      delayMs: 100,
      minimumMs: 300,
    });

    loading.value = true;
    await nextTick();
    await vi.advanceTimersByTimeAsync(50);
    loading.value = false;
    await nextTick();
    await vi.runAllTimersAsync();

    expect(visible.value).toBe(false);
  });

  it("rejects invalid timing configuration", () => {
    expect(() =>
      useMinimumLoading(ref(false), { minimumMs: Number.NaN }),
    ).toThrow(RangeError);
  });
});
