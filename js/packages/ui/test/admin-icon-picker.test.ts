// @vitest-environment happy-dom
import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it, vi } from "vitest";
import AdminIconPicker from "../src/admin/components/AdminIconPicker.vue";

const UButton = defineComponent({
  name: "UButton",
  inheritAttrs: false,
  props: ["icon"],
  emits: ["click"],
  setup(props, { attrs, emit }) {
    return () =>
      h(
        "button",
        { ...attrs, onClick: () => emit("click") },
        props.icon as string,
      );
  },
});
const UTooltip = defineComponent({
  name: "UTooltip",
  props: ["text"],
  setup:
    (_props, { slots }) =>
    () =>
      slots.default?.(),
});
const UInput = defineComponent({
  name: "UInput",
  inheritAttrs: false,
  props: ["modelValue"],
  emits: ["update:modelValue"],
  setup:
    (props, { attrs, emit }) =>
    () =>
      h("input", {
        ...attrs,
        value: props.modelValue,
        onInput: (event: Event) =>
          emit("update:modelValue", (event.target as HTMLInputElement).value),
      }),
});

const global = { components: { UButton, UTooltip, UInput } };

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
      global,
    });
    expect(
      wrapper.get('[aria-label="选择应用"]').attributes("aria-pressed"),
    ).toBe("true");
    await wrapper.get('[aria-label="选择网站"]').trigger("click");
    expect(wrapper.emitted("update:modelValue")).toEqual([
      ["i-tabler-world-www"],
    ]);
    expect(wrapper.get("[data-admin-icon-results]").attributes("class")).toContain(
      "repeat(auto-fill,minmax(2rem,1fr))",
    );
  });

  it("searches the preloaded Tabler management catalog by label and keyword", async () => {
    const wrapper = mount(AdminIconPicker, {
      props: { modelValue: "i-tabler-apps" },
      global,
    });

    await wrapper.get('[aria-label="搜索图标"]').setValue("terminal");

    expect(wrapper.find('[aria-label="选择应用"]').exists()).toBe(false);
    expect(wrapper.find('[aria-label="选择终端"]').exists()).toBe(true);
  });

  it("lazy-loads and searches the complete Tabler catalog", async () => {
    const wrapper = mount(AdminIconPicker, {
      props: { modelValue: "i-tabler-apps" },
      global,
    });

    await flushPromises();
    await wrapper.get('[aria-label="搜索图标"]').setValue("fish");
    await vi.waitFor(
      () => expect(wrapper.find('[aria-label="选择fish"]').exists()).toBe(true),
      { timeout: 5_000 },
    );
  });
});
