// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";
import CollectionSortHeader from "../src/collection/components/CollectionSortHeader.vue";

const UButton = defineComponent({
  name: "UButton",
  inheritAttrs: false,
  props: ["label", "icon"],
  emits: ["click"],
  setup(props, { attrs, emit }) {
    return () =>
      h(
        "button",
        {
          ...attrs,
          "data-icon": props.icon,
          onClick: (event: MouseEvent) => emit("click", event),
        },
        props.label as string,
      );
  },
});

describe("CollectionSortHeader", () => {
  it("owns direction labels, icons and the sort event", async () => {
    const wrapper = mount(CollectionSortHeader, {
      props: { label: "评论日期", active: true, sortOrder: "desc" },
      global: { components: { UButton } },
    });

    const button = wrapper.get("button");
    expect(button.attributes("aria-label")).toContain("当前倒序");
    expect(button.attributes("data-icon")).toBe("i-tabler-arrow-down");

    await button.trigger("click");
    expect(wrapper.emitted("sort")).toHaveLength(1);

    await wrapper.setProps({ sortOrder: "asc" });
    expect(button.attributes("aria-label")).toContain("当前正序");
    expect(button.attributes("data-icon")).toBe("i-tabler-arrow-up");
  });
});
