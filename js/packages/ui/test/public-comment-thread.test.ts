// @vitest-environment happy-dom
import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it, vi } from "vitest";
import PublicCommentThread from "../src/comments/components/PublicCommentThread.vue";
import type { PublicCommentMessages } from "../src/comments/types";

const buttonStub = defineComponent({
  name: "UButton",
  inheritAttrs: false,
  props: { label: String, disabled: Boolean, loading: Boolean },
  setup:
    (props, { attrs, slots }) =>
    () =>
      h(
        "button",
        { ...attrs, disabled: props.disabled || props.loading },
        props.label || slots.default?.(),
      ),
});
const passive = (name: string, tag = "div") =>
  defineComponent({
    name,
    inheritAttrs: false,
    props: { label: String, text: String, alt: String },
    setup:
      (props, { attrs, slots }) =>
      () =>
        h(
          tag,
          attrs,
          props.label || props.text || props.alt || slots.default?.(),
        ),
  });

const messages: PublicCommentMessages = {
  count: (count) => `${count} 条评论`,
  replies: (count) => `${count} 条回复`,
  sort: "评论排序",
  loading: "正在加载评论",
  oldest: "最早",
  newest: "最新",
  reply: "回复",
  cancelReply: "取消回复",
  anonymous: "匿名用户",
  empty: "还没有评论",
  closed: "评论已关闭",
  loadError: "评论加载失败",
  retry: "重试",
  writeComment: "写评论",
  writeReply: "写回复",
  authorName: "昵称",
  authorEmail: "邮箱",
  anonymousHint: "匿名评论需要审核",
  login: "登录",
  submit: "评论",
  submitReply: "回复",
  submitted: "已发布",
  pending: "待审核",
  submitError: "提交失败",
  nameRequired: "请填写昵称",
};

const global = {
  components: {
    UButton: buttonStub,
    UAvatar: passive("UAvatar", "span"),
    UBadge: passive("UBadge", "span"),
    UIcon: passive("UIcon", "span"),
    USkeleton: passive("USkeleton"),
    UAlert: passive("UAlert"),
    UInput: defineComponent({
      name: "UInput",
      inheritAttrs: false,
      props: { modelValue: String, placeholder: String },
      emits: ["update:modelValue"],
      setup:
        (props, { attrs, emit }) =>
        () =>
          h("input", {
            ...attrs,
            value: props.modelValue,
            placeholder: props.placeholder,
            onInput: (event: Event) =>
              emit(
                "update:modelValue",
                (event.target as HTMLInputElement).value,
              ),
          }),
    }),
  },
};

describe("PublicCommentThread", () => {
  it("keeps replies inside the parent surface and owns sorting", async () => {
    const wrapper = mount(PublicCommentThread, {
      props: {
        total: 2,
        order: "asc",
        messages,
        formatTime: () => "刚刚",
        viewer: { authenticated: true, name: "Reader" },
        submit: async () => ({ pending: false }),
        comments: [
          {
            id: "parent",
            authorName: "Alice",
            content: "Parent",
            createdAt: "2026-08-28T00:00:00Z",
            replies: [
              {
                id: "reply",
                parentId: "parent",
                authorName: "Bob",
                content: "Reply",
                createdAt: "2026-08-28T00:01:00Z",
              },
            ],
          },
        ],
      },
      global,
    });

    const thread = wrapper.get("[data-public-comment]");
    expect(
      wrapper.get("[data-public-comment-thread] > header").classes(),
    ).not.toContain("border-b");
    expect(wrapper.get("h2").get('[name="i-tabler-messages"]')).toBeTruthy();
    expect(thread.text()).toContain("Parent");
    expect(thread.text()).toContain("Reply");
    expect(thread.get("time").attributes("datetime")).toBe(
      "2026-08-28T00:00:00Z",
    );
    const newest = wrapper
      .findAll("button")
      .find((button) => button.text() === messages.newest);
    expect(newest).toBeTruthy();
    expect(newest!.attributes("aria-pressed")).toBe("false");
    await newest!.trigger("click");
    expect(wrapper.emitted("update:order")?.at(-1)).toEqual(["desc"]);
  });

  it("submits one inline reply through the caller adapter", async () => {
    const submit = vi.fn(async () => ({ pending: false }));
    const wrapper = mount(PublicCommentThread, {
      props: {
        total: 1,
        messages,
        formatTime: () => "刚刚",
        viewer: { authenticated: true, name: "Reader" },
        submit,
        comments: [
          {
            id: "parent",
            authorName: "Alice",
            content: "Parent",
            createdAt: "2026-08-28T00:00:00Z",
          },
        ],
      },
      global,
    });

    await wrapper.get("[data-public-comment] button").trigger("click");
    const composers = wrapper.findAll("[data-public-comment-composer]");
    expect(composers).toHaveLength(2);
    await composers[0]!.get("textarea").setValue("Inline reply");
    await composers[0]!.get("button").trigger("click");
    await flushPromises();
    expect(submit).toHaveBeenCalledWith({
      content: "Inline reply",
      parentId: "parent",
    });
  });

  it("keeps login available when the product does not allow anonymous comments", async () => {
    const login = vi.fn();
    const wrapper = mount(PublicCommentThread, {
      props: {
        messages,
        formatTime: () => "刚刚",
        viewer: { authenticated: false, name: "" },
        submit: async () => ({ pending: false }),
        login,
      },
      global,
    });

    const loginButton = wrapper
      .findAll("button")
      .find((button) => button.text() === messages.login);
    expect(loginButton).toBeTruthy();
    await loginButton!.trigger("click");
    expect(login).toHaveBeenCalledOnce();
  });
});
