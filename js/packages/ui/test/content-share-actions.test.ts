// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h, nextTick } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ContentShareActions from "../src/sharing/components/ContentShareActions.vue";

const UButton = defineComponent({
  name: "UButton",
  inheritAttrs: false,
  props: {
    label: String,
    to: String,
  },
  emits: ["click"],
  setup(props, { attrs, emit }) {
    return () =>
      h(
        props.to ? "a" : "button",
        {
          ...attrs,
          href: props.to,
          onClick: () => emit("click"),
        },
        props.label,
      );
  },
});

const messages = {
  weibo: "分享到微博",
  x: "分享到 X",
  system: "系统分享",
  copy: "复制链接",
  copied: "已复制",
  copyFailed: "复制失败",
};

beforeEach(() => {
  vi.useFakeTimers();
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
  });
  Object.defineProperty(navigator, "share", {
    configurable: true,
    value: undefined,
  });
});

afterEach(() => {
  vi.useRealTimers();
});

describe("ContentShareActions", () => {
  it("keeps the shared target set small and omits QQ Space", async () => {
    const wrapper = mount(ContentShareActions, {
      props: { title: "文章", url: "https://example.test/post", messages },
      global: { components: { UButton } },
    });
    await nextTick();

    expect(wrapper.find('a[aria-label="分享到微博"]').exists()).toBe(true);
    expect(wrapper.find('a[aria-label="分享到 X"]').exists()).toBe(true);
    expect(wrapper.text()).not.toContain("QQ");
  });

  it("reports clipboard success on the original action and then resets", async () => {
    const wrapper = mount(ContentShareActions, {
      props: { title: "文章", url: "https://example.test/post", messages },
      global: { components: { UButton } },
    });
    await nextTick();

    await wrapper.get("[data-share-copy]").trigger("click");
    await Promise.resolve();
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
      "https://example.test/post",
    );
    expect(wrapper.get("[data-share-copy]").attributes("data-copy-state")).toBe(
      "copied",
    );
    expect(wrapper.get("[data-share-copy]").text()).toContain("已复制");

    vi.advanceTimersByTime(1800);
    await nextTick();
    expect(wrapper.get("[data-share-copy]").text()).toContain("复制链接");
  });
});
