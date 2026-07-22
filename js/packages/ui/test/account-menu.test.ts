// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it, vi } from "vitest";
import AccountMenu, {
  type AccountMenuAppearance,
  type AccountMenuMessages,
} from "../src/account-menu/components/AccountMenu.vue";

const buttonStub = defineComponent({
  inheritAttrs: false,
  setup:
    (_, { attrs, slots }) =>
    () =>
      h("button", attrs, slots.default?.()),
});
const passiveStub = defineComponent({
  inheritAttrs: false,
  props: ["items", "src", "text"],
  setup:
    (props, { attrs, slots }) =>
    () =>
      h("div", { ...attrs, "data-items": JSON.stringify(props.items) }, [
        props.text,
        slots.default?.(),
      ]),
});
const messages: AccountMenuMessages = {
  currentUser: "Current user",
  logout: "Sign out",
  openMenu: (name) => `Open ${name} account menu`,
};
const appearanceMessages = {
  label: "Appearance",
  system: "System",
  light: "Light",
  dark: "Dark",
};
const global = {
  components: {
    UButton: buttonStub,
    UDropdownMenu: passiveStub,
    UAvatar: passiveStub,
    UIcon: passiveStub,
  },
};

describe("AccountMenu", () => {
  it("uses caller-owned fallback and accessible trigger copy", () => {
    const wrapper = mount(AccountMenu, {
      props: { logout: vi.fn(), messages },
      global,
    });
    expect(wrapper.get("button").attributes("aria-label")).toBe(
      "Open Current user account menu",
    );
    expect(wrapper.text()).toContain("Current user");
  });

  it("keeps identity, context, utility and logout actions in separate groups", () => {
    const wrapper = mount(AccountMenu, {
      props: {
        name: "Lin",
        email: "lin@example.test",
        contextActions: [{ label: "Workspace" }],
        utilityActions: [{ label: "Preferences", disabled: true }],
        logout: vi.fn(),
        messages,
      },
      global,
    });
    const dropdown = wrapper.getComponent(passiveStub);
    const groups = dropdown.props("items") as Array<Array<{ label: string }>>;
    expect(groups.map((group) => group.map((item) => item.label))).toEqual([
      ["Lin"],
      ["Workspace"],
      ["Preferences"],
      ["Sign out"],
    ]);
    expect(groups[0]?.[0]).toMatchObject({
      label: "Lin",
      description: "lin@example.test",
      type: "label",
    });
  });

  it("awaits the caller logout command", async () => {
    const logout = vi.fn(async () => undefined);
    const wrapper = mount(AccountMenu, {
      props: { logout, messages },
      global,
    });
    const groups = wrapper.getComponent(passiveStub).props("items") as Array<
      Array<{ onSelect?: () => Promise<void> }>
    >;
    await groups.at(-1)?.[0]?.onSelect?.();
    expect(logout).toHaveBeenCalledOnce();
  });

  it("renders a caller-owned appearance submenu and changes preference", async () => {
    const onChange = vi.fn(async () => undefined);
    const appearance: AccountMenuAppearance = {
      value: "system",
      messages: appearanceMessages,
      onChange,
    };
    const wrapper = mount(AccountMenu, {
      props: { appearance, logout: vi.fn(), messages },
      global,
    });
    const groups = wrapper.getComponent(passiveStub).props("items") as Array<
      Array<{
        label: string;
        children?: Array<{
          label: string;
          checked?: boolean;
          onSelect?: (event: Event) => Promise<void>;
        }>;
      }>
    >;
    expect(groups.map((group) => group.map((item) => item.label))).toEqual([
      ["Current user"],
      ["Appearance"],
      ["Sign out"],
    ]);
    const options = groups[1]?.[0]?.children ?? [];
    expect(options.map(({ label, checked }) => ({ label, checked }))).toEqual([
      { label: "System", checked: true },
      { label: "Light", checked: false },
      { label: "Dark", checked: false },
    ]);

    const event = new Event("select", { cancelable: true });
    await options[2]?.onSelect?.(event);
    expect(event.defaultPrevented).toBe(true);
    expect(onChange).toHaveBeenCalledWith("dark");
  });

  it("owns expanded and collapsed sidebar trigger anatomy", async () => {
    const wrapper = mount(AccountMenu, {
      props: {
        name: "Lin",
        triggerMode: "sidebar",
        logout: vi.fn(),
        messages,
      },
      global,
    });
    expect(wrapper.get("button").classes()).toContain("w-full");
    expect(wrapper.get("button").text()).toContain("Lin");

    await wrapper.setProps({ triggerMode: "collapsed" });
    expect(wrapper.get("button").classes()).toContain("aspect-square");
    expect(wrapper.get("button").text()).not.toContain("Lin");
  });
});
