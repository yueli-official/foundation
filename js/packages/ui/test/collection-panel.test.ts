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
});
