// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";
import type { CollectionPanelMessages } from "../src/collection/panel";
import CollectionPanel from "../src/collection/components/CollectionPanel.vue";

const UInput = defineComponent({
  name: "UInput",
  props: ["modelValue", "placeholder"],
  emits: ["update:modelValue"],
  setup(props, { emit }) {
    return () =>
      h("input", {
        value: props.modelValue,
        placeholder: props.placeholder,
        onInput: (event: Event) =>
          emit("update:modelValue", (event.target as HTMLInputElement).value),
      });
  },
});
const UButton = defineComponent({
  name: "UButton",
  inheritAttrs: false,
  props: ["label"],
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
const UCheckbox = defineComponent({
  name: "UCheckbox",
  inheritAttrs: false,
  props: ["modelValue", "disabled"],
  emits: ["update:modelValue"],
  setup(props, { attrs, emit }) {
    return () =>
      h("input", {
        ...attrs,
        type: "checkbox",
        checked: props.modelValue === true,
        disabled: props.disabled,
        onChange: (event: Event) =>
          emit("update:modelValue", (event.target as HTMLInputElement).checked),
      });
  },
});
const passiveStub = defineComponent({
  inheritAttrs: false,
  setup:
    (_props, { attrs, slots }) =>
    () =>
      h("div", attrs, slots.default?.()),
});
const UPopover = defineComponent({
  name: "UPopover",
  props: { open: Boolean },
  emits: ["update:open"],
  setup:
    (props, { slots }) =>
    () =>
      h("div", { "data-popover": "" }, [
        slots.default?.(),
        props.open ? slots.content?.() : undefined,
      ]),
});

const messages: CollectionPanelMessages = {
  searchPlaceholder: "Search records",
  searchAction: "Search",
  filtersAction: "Filters",
  activeFilters: (count) => `Filters ${count}`,
  clearFilters: "Clear",
  selectPage: "Select page",
  selectItem: (label) => `Select ${label}`,
  bulkRegion: "Bulk actions",
  selected: (count) => `${count} selected`,
  selectAllResults: "Select all",
  clearSelection: "Cancel",
  emptyTitle: "No results",
  emptyDescription: "Change filters",
  errorTitle: "Load failed",
  retry: "Retry",
  showing: (first, last, total) => `${first}-${last}/${total}`,
  pageSize: "Per page",
  pageSizeControl: "Items per page",
  pageSizeOption: (size) => `${size}`,
};

describe("CollectionPanel", () => {
  it("owns search, item selection and result anatomy through one Interface", async () => {
    const items = [
      { id: "one", title: "First" },
      { id: "two", title: "Second" },
    ];
    const wrapper = mount(CollectionPanel, {
      props: {
        label: "Records",
        items,
        itemKey: (item: unknown) => (item as (typeof items)[number]).id,
        itemLabel: (item: unknown) => (item as (typeof items)[number]).title,
        messages,
        total: 2,
        page: 1,
        pageSize: 10,
        selectable: true,
        isItemSelectable: (item: unknown) =>
          (item as (typeof items)[number]).id !== "two",
        isSelected: (key: string | number) => key === "two",
      },
      slots: {
        columns: "Name",
        item: ({ item }: { item: unknown }) =>
          (item as (typeof items)[number]).title,
      },
      global: {
        components: {
          UInput,
          UButton,
          UCheckbox,
          USelect: passiveStub,
          USelectMenu: passiveStub,
          UPopover,
          UPagination: passiveStub,
          USkeleton: passiveStub,
          UIcon: passiveStub,
        },
      },
    });

    await wrapper
      .get('input[placeholder="Search records"]')
      .setValue(" first ");
    await wrapper.get("form").trigger("submit");
    expect(wrapper.emitted("search")).toEqual([["first"]]);
    expect(wrapper.text()).toContain("First");
    expect(wrapper.text()).toContain("Second");
    expect(wrapper.findAll("article")[1]?.classes()).not.toContain(
      "bg-primary/5",
    );
    expect(
      wrapper.get('input[aria-label="Select Second"]').attributes("disabled"),
    ).toBeDefined();

    await wrapper.get('input[aria-label="Select First"]').setValue(true);
    expect(wrapper.emitted("toggleItem")).toEqual([["one", true]]);
    expect(wrapper.text()).toContain("1-2/2");
  });

  it("keeps columns visible and replaces the toolbar in selection mode", async () => {
    const wrapper = mount(CollectionPanel, {
      props: {
        label: "Records",
        items: [{ id: "one", title: "First" }],
        itemKey: (item: unknown) => (item as { id: string; title: string }).id,
        itemLabel: (item: unknown) =>
          (item as { id: string; title: string }).title,
        messages,
        total: 1,
        page: 1,
        pageSize: 10,
        selectable: true,
        selectionCount: 0,
      },
      slots: {
        columns: "Name",
        item: "First",
        "bulk-actions": "Archive",
      },
      global: {
        components: {
          UInput,
          UButton,
          UCheckbox,
          USelect: passiveStub,
          USelectMenu: passiveStub,
          UPopover,
          UPagination: passiveStub,
          USkeleton: passiveStub,
          UIcon: passiveStub,
        },
      },
    });

    expect(wrapper.find("[data-collection-columns]").exists()).toBe(true);
    expect(wrapper.find("[data-collection-table-default]").exists()).toBe(true);
    expect(wrapper.find("[data-collection-table-selection]").exists()).toBe(
      false,
    );

    await wrapper.setProps({ selectionCount: 1 });
    expect(wrapper.find("[data-collection-columns]").exists()).toBe(true);
    expect(wrapper.find("[data-collection-table-default]").exists()).toBe(
      false,
    );
    expect(wrapper.get("[data-collection-table-selection]").text()).toContain(
      "1 selected",
    );
    expect(wrapper.html()).not.toMatch(/\b(?:sticky|fixed)\b/);
    expect(wrapper.get("[data-collection-table-selection]").text()).toContain(
      "Archive",
    );
  });

  it("uses a searchable Nuxt UI control only when caller text is supplied", () => {
    const wrapper = mount(CollectionPanel, {
      props: {
        label: "Records",
        items: [],
        itemKey: () => "record",
        itemLabel: () => "Record",
        messages,
        total: 0,
        page: 1,
        pageSize: 10,
        filtersOpen: true,
        controls: [
          {
            kind: "select",
            id: "category",
            label: "Category",
            value: "",
            options: [{ label: "All", value: "" }],
            searchPlaceholder: "Search categories",
          },
        ],
      },
      global: {
        components: {
          UInput,
          UButton,
          UCheckbox,
          USelect: passiveStub,
          USelectMenu: defineComponent({
            name: "USelectMenu",
            inheritAttrs: false,
            props: ["searchInput"],
            setup: (props, { attrs }) => () =>
              h("div", {
                ...attrs,
                "data-search-placeholder": (
                  props.searchInput as { placeholder?: string }
                )?.placeholder,
              }),
          }),
          UPopover,
          UPagination: passiveStub,
          USkeleton: passiveStub,
          UIcon: passiveStub,
        },
      },
    });

    expect(
      wrapper.get('[data-search-placeholder="Search categories"]'),
    ).toBeTruthy();
    expect(wrapper.get("[data-collection-inline-filter]")).toBeTruthy();
    expect(
      wrapper.findAll("button").some((button) => button.text() === "Filters"),
    ).toBe(false);
  });

  it("keeps two or more select controls inside the filter popover", () => {
    const wrapper = mount(CollectionPanel, {
      props: {
        label: "Records",
        items: [],
        itemKey: () => "record",
        itemLabel: () => "Record",
        messages,
        total: 0,
        page: 1,
        pageSize: 10,
        filtersOpen: true,
        controls: [
          {
            kind: "select",
            id: "status",
            label: "Status",
            value: "all",
            options: [{ label: "All", value: "all" }],
          },
          {
            kind: "select",
            id: "category",
            label: "Category",
            value: "all",
            options: [{ label: "All", value: "all" }],
          },
        ],
      },
      global: {
        components: {
          UInput,
          UButton,
          UCheckbox,
          USelect: passiveStub,
          USelectMenu: passiveStub,
          UPopover,
          UPagination: passiveStub,
          USkeleton: passiveStub,
          UIcon: passiveStub,
        },
      },
    });

    expect(
      wrapper.findAll("button").some((button) => button.text() === "Filters"),
    ).toBe(true);
    expect(wrapper.find("[data-collection-inline-filter]").exists()).toBe(
      false,
    );
    expect(wrapper.text()).toContain("Status");
    expect(wrapper.text()).toContain("Category");
  });

  it("keeps one caller-owned view switch reachable without inventing filters", () => {
    const wrapper = mount(CollectionPanel, {
      props: {
        label: "Records",
        items: [],
        itemKey: () => "record",
        itemLabel: () => "Record",
        messages,
        total: 0,
        page: 1,
        pageSize: 10,
      },
      slots: {
        view: () => h("button", { "aria-label": "Grid" }, "Grid"),
      },
      global: {
        components: {
          UInput,
          UButton,
          UCheckbox,
          USelect: passiveStub,
          USelectMenu: passiveStub,
          UPopover,
          UPagination: passiveStub,
          USkeleton: passiveStub,
          UIcon: passiveStub,
        },
      },
    });

    expect(wrapper.get("[data-collection-table-utilities]").text()).toBe(
      "Grid",
    );
    expect(wrapper.findAll('[aria-label="Grid"]')).toHaveLength(1);
    expect(
      wrapper.findAll("button").some((button) => button.text() === "Filters"),
    ).toBe(false);
  });

  it("does not render an empty filter region when no controls or view are supplied", () => {
    const wrapper = mount(CollectionPanel, {
      props: {
        label: "Records",
        items: [],
        itemKey: () => "record",
        itemLabel: () => "Record",
        messages,
        total: 0,
        page: 1,
        pageSize: 10,
      },
      global: {
        components: {
          UInput,
          UButton,
          UCheckbox,
          USelect: passiveStub,
          USelectMenu: passiveStub,
          UPopover,
          UPagination: passiveStub,
          USkeleton: passiveStub,
          UIcon: passiveStub,
        },
      },
    });

    expect(
      wrapper.findAll("button").some((button) => button.text() === "Filters"),
    ).toBe(false);
    expect(wrapper.find('[id^="y-collection-controls-"]').exists()).toBe(false);
  });
});
