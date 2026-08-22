import { InlineMath } from "@tiptap/extension-mathematics";
import { VueNodeViewRenderer } from "@tiptap/vue-3";
import EditorInlineMathNode from "../components/EditorInlineMathNode.vue";
import { asNodeViewComponent } from "./nodeViewComponent";

// Keep the official commands, Markdown tokenizer and serializer, but own the
// interaction so a rendered inline formula can return to an editable state.
export const EditableInlineMath = InlineMath.extend({
  addAttributes() {
    return {
      ...(this.parent?.() ?? {}),
      editing: { default: false, rendered: false },
    };
  },

  addNodeView() {
    return VueNodeViewRenderer(asNodeViewComponent(EditorInlineMathNode));
  },
});
