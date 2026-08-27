// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";
import AdminIconPicker from "../src/admin/components/AdminIconPicker.vue";

const UButton = defineComponent({
  name: "UButton",
  inheritAttrs: false,
  props: ["icon"],
  emits: ["click"],
  setup(props, { attrs, emit }) {
    return () => h("button", { ...attrs, onClick: () => emit("click") }, props.icon as string);
  },
});
const UTooltip = defineComponent({
  name: "UTooltip",
  setup: (_props, { slots }) => () => slots.default?.(),
});

describe("AdminIconPicker", () => {
  it("exposes a curated accessible choice instead of a raw icon field", async () => {
    const wrapper = mount(AdminIconPicker, {
      props: {
        modelValue: "i-tabler-apps",
        options: [
          { label: "应用", value: "i-tabler-apps" },
          { label: "网站", value: "i-tabler-world-www" },
        ],
      },
      global: { components: { UButton, UTooltip } },
    });
    expect(wrapper.get('[aria-label="选择应用"]').attributes("aria-pressed")).toBe("true");
    await wrapper.get('[aria-label="选择网站"]').trigger("click");
    expect(wrapper.emitted("update:modelValue")).toEqual([["i-tabler-world-www"]]);
  });
});
