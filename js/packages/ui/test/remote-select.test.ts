// @vitest-environment happy-dom
import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";
import RemoteSelect from "../src/remote-select/components/RemoteSelect.vue";
import type {
  RemoteSelectLoadRequest,
  RemoteSelectLoadResult,
  RemoteSelectMessages,
} from "../src/remote-select/types";

const messages: RemoteSelectMessages = {
  placeholder: "Select user",
  searchPlaceholder: "Search users",
  empty: "No users",
  error: "Could not load users",
  retry: "Retry",
  minimumQuery: (count) => `Enter ${count} characters`,
};

const USelectMenu = defineComponent({
  name: "USelectMenu",
  inheritAttrs: false,
  props: {
    modelValue: [String, Number],
    searchTerm: String,
    open: Boolean,
    items: { type: Array, default: () => [] },
    searchInput: Object,
  },
  emits: ["update:modelValue", "update:searchTerm", "update:open"],
  setup(props, { emit, slots }) {
    return () =>
      h("div", [
        h("button", {
          "data-open": "",
          onClick: () => emit("update:open", !props.open),
        }),
        h("input", {
          "data-search": "",
          value: props.searchTerm,
          onInput: (event: Event) =>
            emit("update:searchTerm", (event.target as HTMLInputElement).value),
        }),
        ...(
          props.items as Array<{ value: string | number; label: string }>
        ).map((item) => h("span", { "data-option": item.value }, item.label)),
        props.open && props.items.length === 0 ? slots.empty?.() : undefined,
      ]);
  },
});
const UButton = defineComponent({
  name: "UButton",
  props: { label: String },
  emits: ["click"],
  setup:
    (props, { emit }) =>
    () =>
      h("button", { onClick: () => emit("click") }, props.label),
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

afterEach(() => vi.useRealTimers());

describe("RemoteSelect", () => {
  it("debounces requests, aborts stale work and only displays the latest result", async () => {
    vi.useFakeTimers();
    const requests: Array<{
      request: RemoteSelectLoadRequest;
      result: ReturnType<typeof deferred<RemoteSelectLoadResult>>;
    }> = [];
    const load = vi.fn((request: RemoteSelectLoadRequest) => {
      const result = deferred<RemoteSelectLoadResult>();
      requests.push({ request, result });
      return result.promise;
    });
    const wrapper = mount(RemoteSelect, {
      props: { load, messages, debounceMs: 20 },
      global: { components: { USelectMenu, UButton } },
    });

    await wrapper.get("[data-open]").trigger("click");
    await wrapper.get("[data-search]").setValue("a");
    await vi.advanceTimersByTimeAsync(20);
    expect(load).toHaveBeenCalledTimes(1);
    expect(requests[0]?.request.query).toBe("a");

    await wrapper.get("[data-search]").setValue("al");
    await vi.advanceTimersByTimeAsync(20);
    expect(load).toHaveBeenCalledTimes(2);
    expect(requests[0]?.request.signal.aborted).toBe(true);

    requests[0]?.result.resolve({
      items: [{ value: "stale", label: "Stale" }],
    });
    requests[1]?.result.resolve({
      items: [{ value: "latest", label: "Latest" }],
    });
    await flushPromises();

    expect(wrapper.text()).toContain("Latest");
    expect(wrapper.text()).not.toContain("Stale");
  });

  it("shows caller-owned failure copy and retries the current query", async () => {
    vi.useFakeTimers();
    const load = vi
      .fn()
      .mockRejectedValueOnce(new Error("private transport detail"))
      .mockResolvedValueOnce({
        items: [{ value: 7, label: "Recovered user" }],
      });
    const wrapper = mount(RemoteSelect, {
      props: { load, messages, debounceMs: 0 },
      global: { components: { USelectMenu, UButton } },
    });

    await wrapper.get("[data-open]").trigger("click");
    await vi.runAllTimersAsync();
    await flushPromises();
    expect(wrapper.text()).toContain("Could not load users");
    expect(wrapper.text()).not.toContain("private transport detail");

    await wrapper.get("button:not([data-open])").trigger("click");
    await flushPromises();
    expect(load).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain("Recovered user");
  });
});
