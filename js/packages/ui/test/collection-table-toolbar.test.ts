// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";
import CollectionTableToolbar from "../src/collection/components/CollectionTableToolbar.vue";

const UInput = defineComponent({
  name: "UInput",
  inheritAttrs: false,
  props: ["modelValue", "placeholder"],
  emits: ["update:modelValue"],
  setup(props, { attrs, emit }) {
    return () =>
      h("input", {
        ...attrs,
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

describe("CollectionTableToolbar", () => {
  it("does not publish an IME search value until composition ends", async () => {
    const wrapper = mount(CollectionTableToolbar, {
      props: {
        label: "Document controls",
        search: "",
        searchPlaceholder: "Search documents",
        filterLabel: "Filters",
      },
      global: { components: { UInput, UButton, UPopover } },
    });
    const input = wrapper.get("input");

    await input.trigger("compositionstart");
    await input.setValue("ce s");

    expect(wrapper.emitted("update:search")).toBeUndefined();

    input.element.value = "测试";
    await input.trigger("compositionend");

    expect(wrapper.emitted("update:search")).toEqual([["测试"]]);
  });

  it("owns one deterministic default toolbar with compact filter popover", () => {
    const wrapper = mount(CollectionTableToolbar, {
      props: {
        label: "Document controls",
        search: "",
        searchPlaceholder: "Search documents",
        searchAction: "Search",
        filterLabel: "Filters",
        filterCount: 2,
        selectionCount: 0,
        filtersOpen: true,
      },
      slots: {
        filters: "<select aria-label='Status'><option>All</option></select>",
        utilities: "<button type='button'>Columns</button>",
        "active-filters": "<button type='button'>Status: Published</button>",
        selection: "<span>2 selected</span>",
      },
      global: { components: { UInput, UButton, UPopover } },
    });

    expect(wrapper.get("[data-collection-table-toolbar]")).toBeTruthy();
    expect(wrapper.get("[data-collection-table-default]")).toBeTruthy();
    expect(wrapper.get("[data-collection-table-search]")).toBeTruthy();
    expect(wrapper.get("[data-collection-table-controls]")).toBeTruthy();
    expect(
      wrapper.get("[data-collection-table-filter-panel]").text(),
    ).toContain("All");
    expect(wrapper.get("[data-collection-table-utilities]").text()).toContain(
      "Columns",
    );
    expect(
      wrapper.get("[data-collection-table-active-filters]").text(),
    ).toContain("Status: Published");
    expect(wrapper.find("[data-collection-table-selection]").exists()).toBe(
      false,
    );
    expect(wrapper.html()).not.toMatch(/\b(?:sticky|fixed)\b/);

    const defaultToolbar = wrapper.get("[data-collection-table-default]");
    expect(defaultToolbar.classes()).toContain(
      "@xl:grid-cols-[minmax(14rem,1fr)_auto]",
    );
    expect(wrapper.get("[data-collection-table-search]").classes()).toContain(
      "@xl:col-span-1",
    );
    expect(wrapper.get("[data-collection-table-controls]").classes()).toContain(
      "@xl:col-span-1",
    );
    expect(defaultToolbar.classes().join(" ")).not.toContain("64rem");
    expect(wrapper.html()).not.toContain("[&>*]:!w-full");
  });

  it("replaces default controls without changing toolbar or active-filter height", async () => {
    const wrapper = mount(CollectionTableToolbar, {
      props: {
        label: "Document controls",
        search: "",
        searchPlaceholder: "Search documents",
        searchAction: "Search",
        filterLabel: "Filters",
        filterCount: 1,
        selectionCount: 2,
      },
      slots: {
        filters: "<select aria-label='Status'><option>All</option></select>",
        utilities: "<button type='button'>Columns</button>",
        "active-filters": "<button type='button'>Status: Draft</button>",
        selection: "<span>2 selected</span>",
      },
      global: { components: { UInput, UButton, UPopover } },
    });

    expect(wrapper.find("[data-collection-table-default]").exists()).toBe(
      false,
    );
    expect(wrapper.find("[data-collection-table-search]").exists()).toBe(false);
    expect(wrapper.find("[data-collection-table-filter-panel]").exists()).toBe(
      false,
    );
    expect(wrapper.find("[data-collection-table-utilities]").exists()).toBe(
      false,
    );
    expect(wrapper.get("[data-collection-table-active-filters]").text()).toContain(
      "Status: Draft",
    );
    const selection = wrapper.get("[data-collection-table-selection]");
    expect(selection.text()).toContain("2 selected");
    expect(selection.classes()).toContain("min-h-24");
    expect(selection.classes()).toContain("@xl:min-h-16");
    expect(wrapper.html()).not.toMatch(/\b(?:sticky|fixed)\b/);
  });
});
