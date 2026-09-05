// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";
import MobileBottomNav from "../src/navigation/components/MobileBottomNav.vue";
import type { MobileBottomNavItem } from "../src/navigation/mobile-bottom-nav.types";

const items: readonly MobileBottomNavItem[] = [
  { id: "home", label: "Home", icon: "i-tabler-home", to: "/", active: true },
  { id: "create", label: "Create", icon: "i-tabler-plus" },
  {
    id: "inbox",
    label: "Inbox",
    icon: "i-tabler-mail",
    to: "/inbox",
    badge: 3,
    ariaLabel: "Inbox, 3 unread",
  },
];
const ULink = defineComponent({
  inheritAttrs: false,
  props: ["as", "to", "disabled", "active", "raw"],
  setup:
    (props, { attrs, slots }) =>
    () =>
      h(
        props.as,
        { ...attrs, href: props.to, disabled: props.disabled },
        slots.default?.(),
      ),
});
const global = {
  components: {
    ULink,
    UIcon: defineComponent({ setup: () => () => h("span") }),
  },
};

describe("MobileBottomNav", () => {
  it("distinguishes route destinations from actions and preserves accessible badge meaning", async () => {
    const wrapper = mount(MobileBottomNav, {
      props: { items, label: "Primary" },
      global,
    });
    expect(wrapper.get("nav").attributes("aria-label")).toBe("Primary");
    expect(wrapper.get('a[href="/"]').attributes("aria-current")).toBe("page");
    const action = wrapper.get("button");
    expect(action.attributes("type")).toBe("button");
    expect(action.attributes("aria-current")).toBeUndefined();
    await action.trigger("click");
    expect(wrapper.emitted("select")).toEqual([[items[1]]]);
    expect(wrapper.get('a[href="/inbox"]').attributes("aria-label")).toBe(
      "Inbox, 3 unread",
    );
    await wrapper.setProps({
      items: items.map((item) =>
        item.to ? { ...item, active: item.id === "inbox" } : item,
      ),
    });
    expect(
      wrapper.get('a[href="/"]').attributes("aria-current"),
    ).toBeUndefined();
    expect(wrapper.get('a[href="/inbox"]').attributes("aria-current")).toBe(
      "page",
    );
  });

  it("releases occupied space when hidden or empty and does not execute disabled actions", async () => {
    const wrapper = mount(MobileBottomNav, {
      props: { items: [{ ...items[1], disabled: true }], label: "Primary" },
      global,
    });
    await wrapper.get("button").trigger("click");
    expect(wrapper.emitted("select")).toBeUndefined();
    await wrapper.setProps({ hidden: true });
    expect(wrapper.find("nav").exists()).toBe(false);
    expect(wrapper.find("[data-mobile-bottom-spacer]").exists()).toBe(false);
    await wrapper.setProps({ hidden: false, items: [] });
    expect(wrapper.find("nav").exists()).toBe(false);
  });
});
