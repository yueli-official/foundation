// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it } from "vitest";
import CollectionPagination from "../src/collection/components/CollectionPagination.vue";
import CollectionPaginationBar from "../src/collection/components/CollectionPaginationBar.vue";
const PaginationStub = defineComponent({ props: ["page", "total", "itemsPerPage", "showEdges", "siblingCount"], emits: ["update:page"], template: "<div />" });
const SelectStub = defineComponent({ props: ["modelValue", "items"], emits: ["update:modelValue"], template: "<button />" });
describe("shared pagination", () => {
  it("retains bounded page navigation and folds long ranges", () => {
    const wrapper = mount(CollectionPagination, { props: { modelValue: 50, totalPages: 100, compact: true }, global: { components: { UPagination: PaginationStub } } });
    const pages = wrapper.findComponent(PaginationStub);
    expect(pages.props()).toMatchObject({ total: 100, itemsPerPage: 1, showEdges: true, siblingCount: 1 });
    pages.vm.$emit("update:page", 0);
    pages.vm.$emit("update:page", 101);
    expect(wrapper.emitted("update:modelValue")).toBeUndefined();
    pages.vm.$emit("update:page", 100);
    expect(wrapper.emitted("update:modelValue")).toEqual([[100]]);
  });
  it("accepts only declared page sizes and omits result summaries", () => {
    const wrapper = mount(CollectionPaginationBar, { props: { page: 1, pageSize: 20, total: 105, pageSizes: [20, 40, 60] }, global: { components: { USelect: SelectStub, UPagination: PaginationStub } } });
    const select = wrapper.findComponent(SelectStub);
    select.vm.$emit("update:modelValue", 0);
    select.vm.$emit("update:modelValue", 999);
    select.vm.$emit("update:modelValue", 20);
    expect(wrapper.emitted("pageSizeChange")).toBeUndefined();
    select.vm.$emit("update:modelValue", 40);
    expect(wrapper.emitted("pageSizeChange")).toEqual([[40]]);
    expect(wrapper.text()).not.toMatch(/105|显示|共/);
    expect(wrapper.findComponent(PaginationStub).props("total")).toBe(6);
  });
});
