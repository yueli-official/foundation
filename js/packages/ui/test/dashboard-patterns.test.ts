// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";
import DashboardLayout, {
  type DashboardMessages,
} from "../src/dashboard/components/DashboardLayout.vue";
import PageHeader from "../src/dashboard/components/PageHeader.vue";

const iconStub = defineComponent({
  inheritAttrs: false,
  setup:
    (_, { attrs }) =>
    () =>
      h("span", attrs),
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
      props: { title: "Workspace", description: "Today at a glance" },
      slots: { actions: "Create" },
    });
    expect(wrapper.get("h1").text()).toBe("Workspace");
    expect(wrapper.text()).toContain("Today at a glance");
    expect(wrapper.text()).toContain("Create");
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
      global: { components: { UIcon: iconStub } },
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
      global: { components: { UIcon: iconStub } },
    });
    expect(wrapper.get("h2").text()).toBe("Continue editing");
    expect(wrapper.text()).toContain("Open a collection");
  });
});
