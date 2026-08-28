// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";
import AdminPage from "../src/admin/components/AdminPage.vue";
import AdminConsoleLayout from "../src/admin/components/AdminConsoleLayout.vue";
import ManagePage from "../src/admin/components/ManagePage.vue";
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
  props: {
    ui: { type: Object, default: () => ({}) },
    resizable: Boolean,
    collapsible: Boolean,
  },
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
  props: {
    items: { type: Array, default: () => [] },
    ui: { type: Object, default: () => ({}) },
  },
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
  props: { label: String, variant: String },
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
        slots.left?.(),
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
    UIcon: passthrough("UIcon", "span"),
    NuxtLink: defineComponent({
      name: "NuxtLink",
      inheritAttrs: false,
      props: { to: String },
      setup: (props, { attrs, slots }) => () =>
        h("a", { ...attrs, href: props.to }, slots.default?.()),
    }),
  },
  stubs: { BackToTop: true },
};

describe("admin template", () => {
  it("owns the complete reusable console shell and route canvas", () => {
    const wrapper = mount(AdminConsoleLayout, {
      props: {
        brandLabel: "Docs",
        brandIcon: "i-tabler-book",
        brandTo: "/",
        navigation: [
          { label: "Overview", to: "/admin", active: true },
          { label: "Members", to: "/admin/members" },
        ],
        searchGroups: [{ id: "pages", label: "Pages", items: [] }],
        messages: {
          skipToContent: "Skip",
          search: "Search console",
          searchPlaceholder: "Search pages",
        },
        storageKey: "docs-admin",
        mainId: "docs-main",
        backToTopLabel: "Top",
      },
      slots: { account: "Account", default: "Workspace" },
      global,
    });

    expect(wrapper.get("[data-admin-console]")).toBeTruthy();
    expect(wrapper.get("[data-admin-console-brand-icon]")).toBeTruthy();
    expect(wrapper.get("[data-admin-console-breadcrumb]").text()).toContain(
      "Docs",
    );
    expect(wrapper.get("[data-admin-console-breadcrumb]").text()).toContain(
      "Overview",
    );
    expect(wrapper.get("[data-admin-console-canvas]").text()).toBe(
      "Workspace",
    );
    expect(wrapper.text()).toContain("Account");
    const sidebar = wrapper.findComponent({ name: "UDashboardSidebar" });
    expect(sidebar.props("resizable")).toBe(false);
    expect(sidebar.props("collapsible")).toBe(false);
    expect(sidebar.attributes("class")).toContain(
      "yueli-admin-shell-surface",
    );
    expect(sidebar.attributes("class")).not.toContain("bg-elevated/45");
    expect(sidebar.attributes("class")).not.toContain("bg-default");
  });

  it("owns the dashboard shell and navigation wiring without global search", () => {
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
    expect(wrapper.get("[data-admin-sidebar-brand]").text()).toBe(
      "Neutral Admin",
    );
    expect(wrapper.get("[data-admin-sidebar-primary]").text()).toContain(
      "Home",
    );
    expect(wrapper.get("[data-admin-sidebar-support]").text()).toContain(
      "Help",
    );
    expect(wrapper.get("[data-admin-sidebar-account]").text()).toBe("Account");
    expect(wrapper.get('[role="complementary"]')).toBeTruthy();
    expect(wrapper.get('nav[aria-label="Home"]')).toBeTruthy();
    expect(wrapper.get('nav[aria-label="Help"]')).toBeTruthy();
    expect(wrapper.findComponent({ name: "UDashboardSearchButton" }).exists()).toBe(false);
    expect(wrapper.findComponent({ name: "UDashboardSearch" }).exists()).toBe(false);
    expect(wrapper.get("[data-main-id]").attributes("data-main-id")).toBe(
      "workspace-main",
    );
    const sidebar = wrapper.findComponent({ name: "UDashboardSidebar" });
    expect(sidebar.attributes("class")).toContain("bg-default");
    expect(sidebar.attributes("class")).not.toContain("bg-elevated/45");
  });

  it("offers an opt-in commercial sidebar without framing brand and account", () => {
    const wrapper = mount(AdminShell, {
      props: {
        navigation: [{ label: "Home", to: "/" }],
        searchGroups: [{ id: "pages", label: "Pages", items: [] }],
        sidebarAppearance: "commercial",
        ui: {
          sidebar: "h-svh bg-default",
          sidebarBody: "min-h-0 overflow-y-auto",
          navigationLink: "rounded-xl",
          navigationIcon: "size-8",
        },
        messages: {
          skipToContent: "Skip to content",
          search: "Search",
          searchPlaceholder: "Search pages",
        },
      },
      slots: {
        brand: "Commercial Admin",
        "sidebar-footer": "Account",
      },
      global,
    });

    expect(
      wrapper.get('[data-admin-sidebar-appearance="commercial"]'),
    ).toBeTruthy();
    expect(wrapper.get("[data-admin-sidebar-brand]").classes()).toContain(
      "w-full",
    );
    expect(wrapper.get("[data-admin-sidebar-brand]").classes()).not.toContain(
      "ring",
    );
    expect(wrapper.get("[data-admin-sidebar-account]").classes()).not.toContain(
      "ring",
    );
    expect(
      wrapper.get('[data-admin-sidebar-appearance="commercial"]').classes(),
    ).not.toContain("border-e");
    expect(
      String(
        wrapper.findComponent({ name: "UDashboardSidebar" }).props("ui").footer,
      ).split(/\s+/),
    ).not.toContain("border-t");
    const sidebar = wrapper.findComponent({ name: "UDashboardSidebar" });
    expect(sidebar.attributes("class")).toContain("h-svh");
    expect(String(sidebar.props("ui").body)).toContain("min-h-0");
    const navigation = wrapper.findComponent({ name: "UNavigationMenu" });
    expect(String(navigation.props("ui").link)).toContain("rounded-xl");
    expect(String(navigation.props("ui").linkLeadingIcon)).toContain("size-8");
  });

  it("derives active and initially open state for a two-level navigation group", () => {
    const wrapper = mount(AdminShell, {
      props: {
        navigation: [
          {
            label: "Settings",
            type: "trigger",
            children: [
              { label: "Profile", to: "/settings/profile" },
              {
                label: "Appearance",
                to: "/settings/appearance",
                active: true,
              },
            ],
          },
        ],
        messages: {
          skipToContent: "Skip to content",
          search: "Search",
          searchPlaceholder: "Search pages",
        },
      },
      global,
    });

    expect(
      wrapper.findComponent({ name: "UNavigationMenu" }).props("items")[0],
    ).toMatchObject({ active: true, defaultOpen: true, type: "trigger" });
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

  it("owns the canvas page heading, icon, actions and vertical rhythm", () => {
    const wrapper = mount(ManagePage, {
      props: {
        id: "images",
        title: "Images",
        icon: "i-tabler-photo",
        bodyClass: "results-grid",
      },
      attrs: { "data-gallery-page": "" },
      slots: {
        subtitle: "Optional context",
        actions: "Upload",
        default: "Results",
        footer: "Pagination",
      },
      global,
    });

    const page = wrapper.get("[data-manage-page]");
    expect(page.attributes("aria-labelledby")).toBe("images-title");
    expect(page.classes()).toContain("space-y-5");
    expect(page.attributes()).toHaveProperty("data-gallery-page");
    expect(wrapper.get("[data-manage-page-icon]")).toBeTruthy();
    expect(wrapper.get("#images-title").text()).toBe("Images");
    expect(wrapper.get("[data-manage-page-actions]").text()).toBe("Upload");
    const body = wrapper.get(".results-grid");
    expect(body.text()).toBe("Results");
    expect(body.classes()).toContain("space-y-5");
    expect(wrapper.get("footer").text()).toBe("Pagination");
  });
});
