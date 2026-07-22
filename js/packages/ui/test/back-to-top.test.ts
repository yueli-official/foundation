// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h, nextTick } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import BackToTop from "../src/navigation/components/BackToTop.vue";

const UIcon = defineComponent({
  name: "UIcon",
  setup: () => () => h("span", { "data-icon": "arrow-up" }),
});

beforeEach(() => {
  Object.defineProperty(window, "innerHeight", {
    value: 100,
    configurable: true,
  });
  Object.defineProperty(window, "scrollY", {
    value: 0,
    writable: true,
    configurable: true,
  });
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: () => ({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }),
  });
  Object.defineProperty(window, "scrollTo", {
    configurable: true,
    value: vi.fn(() => {
      Object.defineProperty(window, "scrollY", {
        value: 0,
        writable: true,
        configurable: true,
      });
    }),
  });
  Object.defineProperty(window, "requestAnimationFrame", {
    configurable: true,
    value: (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    },
  });
});

afterEach(() => {
  document.body.innerHTML = "";
});

describe("BackToTop", () => {
  it("appears after the threshold and centers its icon without pixel offsets", async () => {
    const wrapper = mount(BackToTop, {
      props: { threshold: 1, label: "Back to top" },
      attachTo: document.body,
      global: { components: { UIcon } },
    });
    expect(wrapper.find("button").exists()).toBe(false);

    window.scrollY = 101;
    window.dispatchEvent(new Event("scroll"));
    await nextTick();

    const button = wrapper.get("button");
    expect(button.attributes("aria-label")).toBe("Back to top");
    expect(button.classes()).toContain("place-items-center");
    expect(button.classes()).toContain("p-0");
  });

  it("uses reduced-motion scrolling and returns focus to the caller target", async () => {
    const target = document.createElement("main");
    target.id = "content";
    target.tabIndex = -1;
    document.body.append(target);
    const wrapper = mount(BackToTop, {
      props: { threshold: 0, targetId: "content", label: "Back to top" },
      attachTo: document.body,
      global: { components: { UIcon } },
    });
    await nextTick();

    await wrapper.get("button").trigger("click");
    expect(window.scrollTo).toHaveBeenCalledWith({ top: 0, behavior: "auto" });
    expect(document.activeElement).toBe(target);
  });
});
