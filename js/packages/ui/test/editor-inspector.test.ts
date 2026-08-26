// @vitest-environment happy-dom
import { mount } from "@vue/test-utils";
import { defineComponent, h, nextTick, ref } from "vue";
import { beforeEach, describe, expect, it } from "vitest";
import EditorInspector from "../src/admin/components/EditorInspector.vue";

type MediaListener = (event: MediaQueryListEvent) => void;
let mediaMatches = true;
let mediaListener: MediaListener | undefined;

const slideover = defineComponent({
  name: "USlideover",
  inheritAttrs: false,
  props: {
    open: Boolean,
    title: String,
    modal: Boolean,
    overlay: Boolean,
    dismissible: Boolean,
    ui: { type: Object, default: () => ({}) },
  },
  emits: ["update:open"],
  setup: (props, { attrs, slots }) => () =>
    props.open
      ? h(
          "section",
          {
            ...attrs,
            "data-slideover": "",
            "data-modal": String(props.modal),
            "data-overlay": String(props.overlay),
            "data-dismissible": String(props.dismissible),
            "data-content-class": String(props.ui.content || ""),
            "data-footer-class": String(props.ui.footer || ""),
          },
          [h("h2", props.title), slots.body?.(), slots.footer?.()],
        )
      : null,
});

const host = defineComponent({
  components: { EditorInspector },
  setup() {
    const open = ref(false);
    return { open };
  },
  template: `
    <button data-open @click="open = true">Open</button>
    <EditorInspector v-model:open="open" title="Entry settings">
      <p>Inspector body</p>
      <template #footer><button>Save</button></template>
    </EditorInspector>
  `,
});

beforeEach(() => {
  mediaMatches = true;
  mediaListener = undefined;
  window.matchMedia = (() => ({
    matches: mediaMatches,
    media: "(min-width: 1280px)",
    onchange: null,
    addEventListener: (_type: string, listener: MediaListener) => {
      mediaListener = listener;
    },
    removeEventListener: () => undefined,
    addListener: () => undefined,
    removeListener: () => undefined,
    dispatchEvent: () => true,
  })) as typeof window.matchMedia;
});

describe("EditorInspector", () => {
  it("owns docked and overlay modes behind one v-model interface", async () => {
    const wrapper = mount(host, {
      global: { components: { USlideover: slideover } },
    });

    await wrapper.get("[data-open]").trigger("click");
    await nextTick();
    const docked = wrapper.get("[data-slideover]");
    expect(docked.attributes()).toMatchObject({
      "data-modal": "false",
      "data-overlay": "false",
      "data-dismissible": "false",
    });
    expect(docked.classes()).toContain("y-editor-inspector-surface");
    expect(docked.attributes("data-content-class")).toContain("w-[25rem]");
    expect(docked.attributes("data-footer-class")).toContain("hidden");
    expect(wrapper.get("[data-y-editor-inspector]").attributes("data-inspector-mode")).toBe("docked");
    expect(wrapper.text()).toContain("Inspector body");

    mediaMatches = false;
    mediaListener?.({ matches: false } as MediaQueryListEvent);
    await nextTick();
    expect(wrapper.find("[data-slideover]").exists()).toBe(false);

    await wrapper.get("[data-open]").trigger("click");
    await nextTick();
    const overlay = wrapper.get("[data-slideover]");
    expect(overlay.attributes()).toMatchObject({
      "data-modal": "true",
      "data-overlay": "true",
      "data-dismissible": "true",
    });
    expect(wrapper.get("[data-y-editor-inspector]").attributes("data-inspector-mode")).toBe("overlay");
    expect(wrapper.text()).toContain("Save");
  });
});
