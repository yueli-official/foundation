// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";
import ActionFeedbackButton from "../src/feedback/components/ActionFeedbackButton.vue";

const UButton = defineComponent({
  name: "UButton",
  inheritAttrs: false,
  props: ["label", "icon", "color", "variant", "loading"],
  emits: ["click"],
  setup(props, { attrs, emit }) {
    return () =>
      h(
        "button",
        { ...attrs, onClick: (event: MouseEvent) => emit("click", event) },
        props.label as string,
      );
  },
});

describe("ActionFeedbackButton", () => {
  it("resolves caller-owned message keys and exposes terminal state live", () => {
    const wrapper = mount(ActionFeedbackButton, {
      props: {
        status: "success",
        resolveMessage: ({ key }) =>
          key === "foundation.feedback.action.success" ? "Saved" : undefined,
      },
      global: { components: { UButton } },
    });

    expect(wrapper.text()).toBe("Saved");
    expect(wrapper.get("button").attributes("aria-live")).toBe("polite");
  });

  it("prefers an explicit translated label", () => {
    const wrapper = mount(ActionFeedbackButton, {
      props: { status: "error", errorLabel: "保存失败" },
      global: { components: { UButton } },
    });
    expect(wrapper.text()).toBe("保存失败");
  });
});
