// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";
import AdminPage from "../src/admin/components/AdminPage.vue";
import AdminShell from "../src/admin/components/AdminShell.vue";

const passthrough = (name: string, tag = "div") =>
  defineComponent({
    name,
    inheritAttrs: false,
    setup:
      (_props, { attrs, slots }) =>
      () =>
        h(tag, attrs, slots.default?.()),
  });

const dashboardSidebar = defineComponent({
  name: "UDashboardSidebar",
  inheritAttrs: false,
  setup:
    (_props, { attrs, slots }) =>
    () =>
      h("aside", { ...attrs, "data-sidebar": "" }, [
        slots.header?.({ collapsed: false }),
        slots.default?.({ collapsed: false }),
        slots.footer?.({ collapsed: false }),
      ]),
});
const navigationMenu = defineComponent({
  name: "UNavigationMenu",
  inheritAttrs: false,
  props: { items: { type: Array, default: () => [] } },
  setup:
    (props, { attrs }) =>
    () =>
      h(
        "nav",
        attrs,
        (props.items as Array<{ label?: string }>).map((item) =>
          h("a", item.label),
        ),
      ),
});
const dashboardSearchButton = defineComponent({
  name: "UDashboardSearchButton",
  props: { label: String },
  setup: (props) => () =>
    h("button", { "data-search-button": "" }, props.label),
});
const dashboardSearch = defineComponent({
  name: "UDashboardSearch",
  props: { placeholder: String },
  setup: (props) => () =>
    h("div", { "data-search-placeholder": props.placeholder }),
});
const dashboardPanel = defineComponent({
  name: "UDashboardPanel",
  inheritAttrs: false,
  setup:
    (_props, { attrs, slots }) =>
    () =>
      h("section", { ...attrs, "data-panel": "" }, [
        slots.header?.(),
        slots.body?.(),
        slots.footer?.(),
      ]),
});
const dashboardNavbar = defineComponent({
  name: "UDashboardNavbar",
  props: { title: String },
  setup:
    (props, { slots }) =>
    () =>
      h("header", { "data-navbar": "" }, [
        slots.leading?.(),
        slots.title?.() ?? h("h1", props.title),
        slots.trailing?.(),
        slots.right?.(),
      ]),
});
const dashboardToolbar = defineComponent({
  name: "UDashboardToolbar",
  setup:
    (_props, { slots }) =>
    () =>
      h("div", { "data-toolbar": "" }, [
        slots.left?.(),
        slots.default?.(),
        slots.right?.(),
      ]),
});

const global = {
  components: {
    UDashboardGroup: passthrough("UDashboardGroup"),
    UDashboardSidebar: dashboardSidebar,
    UDashboardSearchButton: dashboardSearchButton,
    UDashboardSearch: dashboardSearch,
    UNavigationMenu: navigationMenu,
    UDashboardPanel: dashboardPanel,
    UDashboardNavbar: dashboardNavbar,
    UDashboardToolbar: dashboardToolbar,
    UDashboardSidebarCollapse: passthrough(
      "UDashboardSidebarCollapse",
      "button",
    ),
  },
};

describe("admin template", () => {
  it("owns the dashboard shell, search and navigation wiring", () => {
    const wrapper = mount(AdminShell, {
      props: {
        navigation: [{ label: "Home", to: "/" }],
        secondaryNavigation: [{ label: "Help", to: "/help" }],
        searchGroups: [{ id: "pages", label: "Pages", items: [] }],
        mainId: "workspace-main",
        messages: {
          skipToContent: "Skip to content",
          search: "Search",
          searchPlaceholder: "Search pages",
        },
      },
      slots: {
        brand: "Neutral Admin",
        "sidebar-footer": "Account",
        default: ({ mainId }: { mainId: string }) =>
          h("div", { "data-main-id": mainId }, "Page"),
      },
      global,
    });

    expect(wrapper.get("[data-admin-shell]")).toBeTruthy();
    expect(wrapper.get('a[href="#workspace-main"]').text()).toBe(
      "Skip to content",
    );
    expect(wrapper.text()).toContain("Neutral Admin");
    expect(wrapper.text()).toContain("Home");
    expect(wrapper.text()).toContain("Help");
    expect(wrapper.text()).toContain("Account");
    expect(wrapper.get('[role="complementary"]')).toBeTruthy();
    expect(wrapper.get('nav[aria-label="Home"]')).toBeTruthy();
    expect(wrapper.get('nav[aria-label="Help"]')).toBeTruthy();
    expect(wrapper.get("[data-search-button]").text()).toBe("Search");
    expect(wrapper.get("[data-search-placeholder]").attributes()).toMatchObject(
      { "data-search-placeholder": "Search pages" },
    );
    expect(wrapper.get("[data-main-id]").attributes("data-main-id")).toBe(
      "workspace-main",
    );
  });

  it("keeps page navbar, toolbar, body and footer under one panel", () => {
    const wrapper = mount(AdminPage, {
      props: { id: "documents", title: "Documents", mainId: "documents-main" },
      slots: {
        actions: "Create",
        "toolbar-left": "Filters",
        "toolbar-right": "Display",
        default: "Results",
        footer: "Pagination",
      },
      global,
    });

    expect(wrapper.get("[data-panel]")).toBeTruthy();
    expect(wrapper.get('[role="main"][aria-label="Documents"]')).toBeTruthy();
    expect(wrapper.get("[data-navbar]").text()).toContain("Documents");
    expect(wrapper.get("[data-admin-page-navbar]")).toBeTruthy();
    expect(wrapper.get("[data-navbar]").text()).toContain("Create");
    expect(wrapper.get("[data-toolbar]").text()).toContain("Filters");
    expect(wrapper.get("[data-toolbar]").text()).toContain("Display");
    expect(wrapper.get("[data-admin-page-toolbar]")).toBeTruthy();
    expect(wrapper.get("#documents-main").text()).toBe("Results");
    expect(wrapper.get("#documents-main").element.tagName).toBe("DIV");
    expect(wrapper.get("#documents-main").classes()).toContain(
      "overflow-y-auto",
    );
    expect(wrapper.text()).toContain("Pagination");
  });
});
