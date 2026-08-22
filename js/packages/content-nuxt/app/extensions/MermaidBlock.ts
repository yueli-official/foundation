import {
  Node,
  mergeAttributes,
  type JSONContent,
  type MarkdownParseHelpers,
  type MarkdownToken,
} from "@tiptap/core";
import { VueNodeViewRenderer } from "@tiptap/vue-3";
import EditorMermaidBlockNode from "../components/EditorMermaidBlockNode.vue";
import { asNodeViewComponent } from "./nodeViewComponent";

// Mermaid 图表块使用 ```mermaid 围栏，并在编辑器中提供实时 SVG 预览。
declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    mermaidBlock: {
      setMermaidBlock: (attrs?: {
        code?: string;
        editing?: boolean;
      }) => ReturnType;
    };
  }
}

export const MermaidBlock = Node.create({
  name: "mermaidBlock",
  group: "block",
  atom: true,
  selectable: true,
  draggable: true,

  addAttributes() {
    return {
      code: { default: "" },
      editing: { default: false, rendered: false },
      previewSvg: { default: "", rendered: false },
      previewCode: { default: "", rendered: false },
      previewError: { default: "", rendered: false },
    };
  },

  parseHTML() {
    return [{ tag: 'div[data-type="mermaid-block"]' }];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, { "data-type": "mermaid-block" }),
    ];
  },

  // @tiptap/markdown 集成。
  markdownTokenName: "mermaidBlock",
  markdownTokenizer: {
    name: "mermaidBlock",
    level: "block" as const,
    start: "```mermaid",
    tokenize(src: string) {
      const match = src.match(/^```mermaid\n([\s\S]+?)```/);
      if (!match) return undefined;
      return {
        type: "mermaidBlock",
        raw: match[0],
        text: (match[1] ?? "").trim(),
      };
    },
  },
  parseMarkdown: (token: MarkdownToken, helpers: MarkdownParseHelpers) => {
    return helpers.createNode("mermaidBlock", { code: token.text || "" });
  },
  renderMarkdown: (node: JSONContent) => {
    const code = String(node.attrs?.code || "");
    return `\`\`\`mermaid\n${code}\n\`\`\``;
  },

  addNodeView() {
    return VueNodeViewRenderer(asNodeViewComponent(EditorMermaidBlockNode));
  },

  addCommands() {
    return {
      setMermaidBlock:
        (attrs?: { code?: string; editing?: boolean }) =>
        ({ commands }) => {
          return commands.insertContent({
            type: this.name,
            attrs: {
              code: attrs?.code ?? "",
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
