// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h, nextTick } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ReadingTableOfContents from "../src/navigation/components/ReadingTableOfContents.vue";

const UIcon = defineComponent({
  name: "UIcon",
  setup: () => () => h("span", { "data-icon": "list-tree" }),
});

beforeEach(() => {
  Object.defineProperty(window, "requestAnimationFrame", {
    configurable: true,
    value: (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    },
  });
  Object.defineProperty(window, "cancelAnimationFrame", {
    configurable: true,
    value: vi.fn(),
  });
});

afterEach(() => {
  document.body.innerHTML = "";
  history.replaceState(null, "", "/");
});

describe("ReadingTableOfContents", () => {
  it("filters heading levels and exposes hierarchy without caller-owned styling", async () => {
    const wrapper = mount(ReadingTableOfContents, {
      props: {
        title: "本页目录",
        minLevel: 2,
        maxLevel: 4,
        items: [
          { id: "title", text: "Title", level: 1 },
          { id: "install", text: "Install", level: 2 },
          { id: "verify", text: "Verify", level: 3 },
          { id: "deep", text: "Deep", level: 5 },
        ],
      },
      global: { components: { UIcon } },
    });
    await nextTick();

    expect(wrapper.get("nav").attributes("aria-label")).toBe("本页目录");
    expect(wrapper.findAll("a")).toHaveLength(2);
    expect(wrapper.findAll("a")[0]?.attributes("data-toc-depth")).toBe("0");
    expect(wrapper.findAll("a")[1]?.attributes("data-toc-depth")).toBe("1");
    expect(wrapper.findAll("a")[1]?.attributes("style")).toContain(
      "padding-left: 20px",
    );
  });

  it("navigates with a real fragment link and marks the selected heading", async () => {
    const heading = document.createElement("h2");
    heading.id = "install";
    heading.scrollIntoView = vi.fn();
    document.body.append(heading);

    const wrapper = mount(ReadingTableOfContents, {
      attachTo: document.body,
      props: { items: [{ id: "install", text: "Install", level: 2 }] },
      global: { components: { UIcon } },
    });
    await wrapper.get("a").trigger("click");

    expect(wrapper.get("a").attributes("href")).toBe("#install");
    expect(wrapper.get("a").attributes("aria-current")).toBe("location");
    expect(heading.scrollIntoView).toHaveBeenCalledWith({
      behavior: "smooth",
      block: "start",
    });
    expect(location.hash).toBe("#install");
  });
});
