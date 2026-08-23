// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";
import DashboardLayout, {
  type DashboardMessages,
} from "../src/dashboard/components/DashboardLayout.vue";
import PageHeader from "../src/dashboard/components/PageHeader.vue";
import TabbedSurface from "../src/dashboard/components/TabbedSurface.vue";

const iconStub = defineComponent({
  inheritAttrs: false,
  setup:
    (_, { attrs }) =>
    () =>
      h("span", attrs),
});
const cardStub = defineComponent({
  name: "UCard",
  inheritAttrs: false,
  props: { variant: String },
  setup:
    (_props, { attrs, slots }) =>
    () =>
      h("section", attrs, [slots.header?.(), slots.default?.()]),
});
const global = { components: { UIcon: iconStub, UCard: cardStub } };
const tabsStub = defineComponent({
  name: "UTabs",
  inheritAttrs: false,
  props: { items: Array, modelValue: String },
  setup: (props, { attrs }) => () =>
    h(
      "div",
      attrs,
      (props.items as Array<{ label: string; value: string }>).map((item) =>
        h("button", { "data-value": item.value }, item.label),
      ),
    ),
});
const messages: DashboardMessages = {
  metrics: "Key metrics",
  pending: { title: "Pending", description: "Needs attention" },
  recent: { title: "Recent work", description: "Continue working" },
  health: { title: "Health", description: "Service status" },
  quickActions: { title: "Quick actions", description: "Common next steps" },
};

describe("dashboard Patterns", () => {
  it("renders caller-owned page header copy and actions", () => {
    const wrapper = mount(PageHeader, {
      props: {
        title: "Workspace",
        icon: "i-tabler-layout-dashboard",
        description: "Today at a glance",
      },
      slots: { actions: "Create" },
      global,
    });
    expect(wrapper.get("h1").text()).toBe("Workspace");
    expect(wrapper.text()).toContain("Today at a glance");
    expect(wrapper.text()).toContain("Create");
    expect(
      wrapper.find('[name="i-tabler-layout-dashboard"]').exists(),
    ).toBe(true);
    expect(wrapper.get("[data-manage-page-actions]").text()).toBe("Create");
  });

  it("keeps same-page workflows inside one shared tabbed surface", () => {
    const wrapper = mount(TabbedSurface, {
      props: {
        modelValue: "library",
        navigationLabel: "Asset sections",
        items: [
          { label: "Library", value: "library" },
          { label: "Storage", value: "storage" },
        ],
      },
      slots: { default: "Active workflow" },
      global: { components: { UTabs: tabsStub } },
    });
    expect(wrapper.classes()).toContain("yueli-card");
    expect(wrapper.get("nav").attributes("aria-label")).toBe("Asset sections");
    expect(wrapper.get("nav").classes()).toContain("overflow-y-hidden");
    expect(wrapper.findAll("button").map((button) => button.text())).toEqual([
      "Library",
      "Storage",
    ]);
    expect(wrapper.text()).toContain("Active workflow");
  });

  it("keeps the complete dashboard decision order and labelled regions", () => {
    const wrapper = mount(DashboardLayout, {
      props: { title: "Dashboard", description: "Decide next", messages },
      slots: {
        metrics: "Metrics",
        pending: "Pending content",
        recent: "Recent content",
        health: "Healthy",
        quickActions: "Create item",
      },
      global,
    });
    expect(wrapper.findAll("h2").map((heading) => heading.text())).toEqual([
      "Key metrics",
      "Pending",
      "Recent work",
      "Health",
      "Quick actions",
    ]);
    expect(
      wrapper
        .get('[data-dashboard-column="primary"]')
        .findAll("h2")
        .map((heading) => heading.text()),
    ).toEqual(["Pending", "Recent work"]);
    expect(
      wrapper
        .get('[data-dashboard-column="secondary"]')
        .findAll("h2")
        .map((heading) => heading.text()),
    ).toEqual(["Health", "Quick actions"]);
    for (const region of wrapper.findAll("section")) {
      const labelledby = region.attributes("aria-labelledby");
      expect(labelledby).toBeTruthy();
      expect(wrapper.find(`#${labelledby}`).exists()).toBe(true);
    }
  });

  it("allows one product to override section copy without forking anatomy", () => {
    const wrapper = mount(DashboardLayout, {
      props: {
        title: "Docs",
        messages,
        recentTitle: "Continue editing",
        recentDescription: "Open a collection",
      },
      slots: { recent: "Documents" },
      global,
    });
    expect(wrapper.get("h2").text()).toBe("Continue editing");
    expect(wrapper.text()).toContain("Open a collection");
  });

  it("offers a quiet commercial surface without stacked panel rules", () => {
    const wrapper = mount(DashboardLayout, {
      props: {
        title: "Dashboard",
        description: "Decide next",
        messages,
        appearance: "commercial",
      },
      slots: {
        pending: "Pending content",
        recent: "Recent content",
        health: "Healthy",
        quickActions: "Create item",
      },
      global,
    });

    expect(
      wrapper.get('[data-dashboard-appearance="commercial"]'),
    ).toBeTruthy();
    const cards = wrapper.findAllComponents({ name: "UCard" });
    expect(cards).toHaveLength(4);
    for (const card of cards) expect(card.props("variant")).toBe("soft");
    const panels = wrapper.findAll("[data-dashboard-panel]");
    expect(panels).toHaveLength(4);
    for (const panel of panels) {
      expect(panel.classes()).not.toContain("border");
      expect(panel.classes()).not.toContain("shadow-sm");
    }
    for (const header of wrapper.findAll("[data-dashboard-panel-header]")) {
      expect(header.classes()).not.toContain("border-b");
    }
  });
});
