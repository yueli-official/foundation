// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";
import SettingSection from "../src/settings/components/SettingSection.vue";
import SettingsLayout from "../src/settings/components/SettingsLayout.vue";
import SettingsSaveDock from "../src/settings/components/SettingsSaveDock.vue";

const passiveStub = defineComponent({
  inheritAttrs: false,
  props: ["label"],
  emits: ["click"],
  setup:
    (props, { attrs, emit, slots }) =>
    () =>
      h(
        "button",
        { ...attrs, onClick: () => emit("click") },
        (props.label as string) || slots.default?.(),
      ),
});

describe("settings Patterns", () => {
  it("renders caller-owned settings copy and section anatomy", async () => {
    const layout = mount(SettingsLayout, {
      props: {
        title: "Workspace",
        description: "Configure it",
        navigationLabel: "Settings sections",
        sections: [
          { key: "general", label: "General" },
          { key: "access", label: "Access" },
        ],
      },
      global: { components: { UButton: passiveStub, USelect: passiveStub } },
    });
    expect(layout.get("h1").text()).toBe("Workspace");
    expect(layout.get('nav[aria-label="Settings sections"]')).toBeTruthy();

    await layout.setProps({ showHeader: false });
    expect(layout.find("h1").exists()).toBe(false);

    const section = mount(SettingSection, {
      props: { title: "Identity", description: "Public profile" },
      slots: { default: "Fields" },
    });
    expect(section.text()).toContain("Identity");
    expect(section.text()).toContain("Fields");
  });

  it("exposes one live save region with caller-owned labels", async () => {
    const wrapper = mount(SettingsSaveDock, {
      attachTo: document.body,
      props: {
        dirty: true,
        messages: {
          region: "Save settings",
          unsaved: "Unsaved changes",
          saving: "Saving changes",
          saved: "Changes saved",
          failed: "Save failed",
          discard: "Discard",
          save: "Save",
          savePending: "Saving",
          saveSuccess: "Saved",
        },
      },
      global: {
        components: { UButton: passiveStub, UIcon: passiveStub },
        stubs: { ActionFeedbackButton: passiveStub },
      },
    });
    const region = document.body.querySelector('[aria-label="Save settings"]');
    expect(region?.textContent).toContain("Unsaved changes");
    const discard = Array.from(document.body.querySelectorAll("button")).find(
      (button) => button.textContent === "Discard",
    );
    discard?.click();
    await wrapper.vm.$nextTick();
    expect(wrapper.emitted("discard")).toHaveLength(1);
    wrapper.unmount();
  });
});
