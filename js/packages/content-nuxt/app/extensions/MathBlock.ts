import {
  Node,
  mergeAttributes,
  type JSONContent,
  type MarkdownParseHelpers,
  type MarkdownToken,
} from "@tiptap/core";
import { VueNodeViewRenderer } from "@tiptap/vue-3";
import EditorMathBlockNode from "../components/EditorMathBlockNode.vue";
import { asNodeViewComponent } from "./nodeViewComponent";

// 块级 LaTeX 公式（$$…$$）由 KaTeX 渲染；行内公式（$…$）交给
// @tiptap/extension-mathematics。
declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    mathBlock: {
      setMathBlock: (attrs?: {
        latex?: string;
        editing?: boolean;
      }) => ReturnType;
    };
  }
}

export const MathBlock = Node.create({
  name: "blockMath",
  group: "block",
  atom: true,
  selectable: true,
  draggable: true,

  addAttributes() {
    return {
      latex: { default: "" },
      editing: { default: false, rendered: false },
    };
  },

  parseHTML() {
    return [{ tag: 'div[data-type="block-math"]' }];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, { "data-type": "block-math" }),
    ];
  },

  // @tiptap/markdown 集成。
  markdownTokenName: "blockMath",
  markdownTokenizer: {
    name: "blockMath",
    level: "block" as const,
    start: "$$",
    tokenize(src: string) {
      const match = src.match(/^\$\$([\s\S]+?)\$\$/);
      if (!match) return undefined;
      return {
        type: "blockMath",
        raw: match[0],
        text: (match[1] ?? "").trim(),
      };
    },
  },
  parseMarkdown: (token: MarkdownToken, helpers: MarkdownParseHelpers) => {
    return helpers.createNode("blockMath", { latex: token.text || "" });
  },
  renderMarkdown: (node: JSONContent) => {
    const latex = String(node.attrs?.latex || "");
    return `$$\n${latex}\n$$`;
  },

  addNodeView() {
    return VueNodeViewRenderer(asNodeViewComponent(EditorMathBlockNode));
  },

  addCommands() {
    return {
      setMathBlock:
        (attrs?: { latex?: string; editing?: boolean }) =>
        ({ commands }) => {
          return commands.insertContent({
            type: this.name,
            attrs: {
              latex: attrs?.latex ?? "",
              editing: attrs?.editing ?? false,
            },
          });
        },
    };
  },

  addInputRules() {
    return [];
  },
});
