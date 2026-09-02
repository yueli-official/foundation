// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { defineComponent, h } from "vue";
import { describe, expect, it, vi } from "vitest";
import CommentModerationCollection from "../src/comments/admin/components/CommentModerationCollection.vue";
import type {
  CommentModerationCollectionActions,
  CommentModerationCollectionModel,
} from "../src/comments/admin/types";

const collectionPanelStub = defineComponent({
  name: "CollectionPanel",
  props: { items: { type: Array, default: () => [] } },
  setup:
    (props, { slots }) =>
    () =>
      h("section", { "data-panel-stub": "" }, [
        slots.navigation?.(),
        h("header", slots.columns?.()),
        ...props.items.map((item) => h("article", slots.item?.({ item }))),
        h("footer", slots["bulk-actions"]?.()),
      ]),
});
const passive = (name: string, tag = "span") =>
  defineComponent({
    name,
    inheritAttrs: false,
    props: { label: String, name: String, text: String },
    setup:
      (props, { attrs, slots }) =>
      () =>
        h(
          tag,
          { ...attrs, "data-icon": props.name },
          props.label || props.text || slots.default?.(),
        ),
  });

describe("CommentModerationCollection", () => {
  it("publishes its nested component directory to Tailwind", () => {
    const tailwind = readFileSync(
      join(process.cwd(), "src/tailwind.css"),
      "utf8",
    );
    expect(tailwind).toContain('@source "./comments/admin/components";');
  });

  it("owns canonical columns and lets products adapt source and actions", async () => {
    const approve = vi.fn();
    const model: CommentModerationCollectionModel = {
      search: "",
      searchPlaceholder: "搜索评论、评论者或图片…",
      items: [
        {
          id: "comment-1",
          content: "测试评论",
          createdAt: "2026-08-28T00:00:00Z",
          authorName: "月离",
          anonymous: true,
          reply: true,
          approve: true,
          actions: [
            {
              id: "delete",
              label: "删除评论",
              icon: "i-tabler-trash",
            },
          ],
          status: { label: "待审核", color: "warning" },
          source: {
            label: "测试图片",
            to: "/images/image-1",
            icon: "i-tabler-photo",
          },
        },
      ],
      state: "ready",
      total: 1,
      page: 1,
      pageSize: 20,
      sortOrder: "desc",
      lifecycle: "trash",
    };
    const actions: CommentModerationCollectionActions = {
      updateSearch: vi.fn(),
      search: vi.fn(),
      controlChange: vi.fn(),
      clearFilters: vi.fn(),
      retry: vi.fn(),
      sort: vi.fn(),
      lifecycleChange: vi.fn(),
      emptyTrash: vi.fn(),
      approve,
      pageChange: vi.fn(),
      pageSizeChange: vi.fn(),
    };
    const wrapper = mount(CommentModerationCollection, {
      props: { model, actions, formatDate: () => "刚刚" },
      global: {
        stubs: {
          CollectionPanel: collectionPanelStub,
          CollectionSortHeader: passive("CollectionSortHeader"),
          CollectionLifecycleTabs: defineComponent({
            name: "CollectionLifecycleTabs",
            props: { items: { type: Array, default: () => [] } },
            setup: (props, { slots }) => {
              const items = props.items as Array<{
                icon?: string;
                label?: string;
              }>;
              return () =>
                h("nav", [
                  ...items.map((item) =>
                    h("button", { type: "button" }, [
                      h("span", { "data-lifecycle-icon": item.icon }),
                      item.label,
                    ]),
                  ),
                  slots.actions?.(),
                ]);
            },
          }),
          AdminRowActions: defineComponent({
            name: "AdminRowActions",
            props: { label: String },
            setup: (props) => () =>
              h("button", { "data-row-actions": "" }, props.label),
          }),
          UButton: defineComponent({
            name: "UButton",
            props: { label: String },
            emits: ["click"],
            setup:
              (props, { emit }) =>
              () =>
                h("button", { onClick: () => emit("click") }, props.label),
          }),
          UModal: defineComponent({
            name: "UModal",
            setup:
              (_props, { slots }) =>
              () =>
                h("div", slots.default?.()),
          }),
          UAvatar: passive("UAvatar"),
          UBadge: passive("UBadge"),
          UIcon: passive("UIcon"),
          NuxtLink: passive("NuxtLink", "a"),
        },
      },
    });

    for (const heading of ["评论", "来源", "用户", "评论日期", "操作"]) {
      expect(wrapper.text()).toContain(heading);
    }
    for (const lifecycle of ["全部", "待审核", "已通过", "垃圾", "回收站"]) {
      expect(wrapper.text()).toContain(lifecycle);
    }
    expect(wrapper.text()).toContain("清空回收站");
    expect(
      wrapper.find('[data-lifecycle-icon="i-tabler-trash"]').exists(),
    ).toBe(true);
    expect(wrapper.text()).toContain("测试评论");
    expect(wrapper.text()).toContain("测试图片");
    expect(wrapper.text()).toContain("匿名用户");
    expect(wrapper.find('[data-icon="i-tabler-photo"]').exists()).toBe(true);
    expect(wrapper.find("[data-row-actions]").exists()).toBe(true);
    const approveButton = wrapper
      .findAll("button")
      .find((button) => button.text() === "通过");
    expect(approveButton).toBeTruthy();
    await approveButton!.trigger("click");
    expect(approve).toHaveBeenCalledWith("comment-1");
  });
});
