// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it, vi } from "vitest";
import AdminRowActions from "../src/admin/components/AdminRowActions.vue";

const passthrough = (name: string, tag = "div") =>
  defineComponent({
    name,
    inheritAttrs: false,
    props: {
      text: String,
      to: String,
      target: String,
      rel: String,
      items: Array,
    },
    setup:
      (_props, { attrs, slots }) =>
      () =>
        h(tag, attrs, slots.default?.()),
  });

const global = {
  components: {
    UTooltip: passthrough("UTooltip"),
    UButton: passthrough("UButton", "button"),
    UIcon: defineComponent({
      name: "UIcon",
      props: { name: String },
      setup:
        (props, { attrs }) =>
        () =>
          h("span", { ...attrs, "data-icon": props.name }),
    }),
    UDropdownMenu: passthrough("UDropdownMenu"),
  },
};

describe("AdminRowActions", () => {
  it("owns the compact size, glyph size, centering and accessible names", async () => {
    const edit = vi.fn();
    const wrapper = mount(AdminRowActions, {
      props: {
        label: "文章操作",
        items: [
          {
            id: "view",
            label: "查看文章",
            icon: "i-tabler-external-link",
            to: "/posts/one",
          },
          {
            id: "edit",
            label: "编辑文章",
            icon: "i-tabler-pencil",
            onSelect: edit,
          },
        ],
      },
      global,
    });

    const actions = wrapper.findAll("[data-admin-row-action]");
    expect(actions).toHaveLength(2);
    for (const action of actions) {
      expect(action.classes()).toEqual(
        expect.arrayContaining([
          "size-11",
          "sm:size-6",
          "items-center",
          "justify-center",
          "p-0",
        ]),
      );
      expect(action.get("[data-icon]").classes()).toEqual(
        expect.arrayContaining(["block", "size-4"]),
      );
      expect(action.attributes("aria-label")).toBeTruthy();
    }
    await actions[1]!.trigger("click");
    expect(edit).toHaveBeenCalledOnce();
  });

  it("owns the vertical overflow trigger and preserves menu groups", () => {
    const wrapper = mount(AdminRowActions, {
      props: {
        label: "更多操作",
        presentation: "overflow",
        items: [
          [{ id: "edit", label: "编辑", icon: "i-tabler-pencil" }],
          [
            {
              id: "delete",
              label: "删除",
              icon: "i-tabler-trash",
              tone: "danger",
            },
          ],
        ],
      },
      global,
    });

    const trigger = wrapper.get("[data-admin-row-action-overflow]");
    expect(trigger.attributes("aria-label")).toBe("更多操作");
    expect(trigger.get('[data-icon="i-tabler-dots-vertical"]')).toBeTruthy();
    expect(
      wrapper.getComponent({ name: "UDropdownMenu" }).props("items"),
    ).toHaveLength(2);
  });
});
