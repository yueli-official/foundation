import {
  Node,
  mergeAttributes,
  type JSONContent,
  type MarkdownParseHelpers,
  type MarkdownToken,
} from "@tiptap/core";
import { VueNodeViewRenderer } from "@tiptap/vue-3";
import EditorCodeBlockNode from "../components/EditorCodeBlockNode.vue";
import { asNodeViewComponent } from "./nodeViewComponent";

// 带语言选择和 highlight.js 预览的代码块。它替换 starter-kit 同名节点，
// 保证 ```lang 围栏在编辑与阅读之间往返不丢失。
export const CodeBlockWithLang = Node.create({
  name: "codeBlock",
  group: "block",
  atom: true,
  selectable: true,
  draggable: true,

  addAttributes() {
    return {
      language: { default: null },
      code: { default: "" },
      editing: { default: false, rendered: false },
    };
  },

  parseHTML() {
    return [
      {
        tag: "pre",
        preserveWhitespace: "full" as const,
        getAttrs: (el: string | HTMLElement) => {
          if (typeof el === "string") return {};
          const code = el.querySelector("code");
          if (!code)
            return { code: typeof el === "string" ? "" : el.textContent || "" };
          const cls = [...code.classList].find((c) =>
            c.startsWith("language-"),
          );
          return {
            language: cls ? cls.replace("language-", "") : null,
            code: code.textContent || "",
          };
        },
      },
    ];
  },

  renderHTML({ node, HTMLAttributes }) {
    const lang = node.attrs.language;
    const codeAttrs = lang ? { class: `language-${lang}` } : {};
    return [
      "pre",
      mergeAttributes(HTMLAttributes),
      ["code", codeAttrs, node.attrs.code || ""],
    ];
  },

  // @tiptap/markdown 集成。
  markdownTokenName: "code",
  parseMarkdown: (token: MarkdownToken, helpers: MarkdownParseHelpers) => {
    if (
      token.raw?.startsWith("```") === false &&
      token.raw?.startsWith("~~~") === false &&
      token.codeBlockStyle !== "indented"
    ) {
      return [];
    }
    return helpers.createNode("codeBlock", {
      language: token.lang || null,
      code: token.text || "",
    });
  },
  renderMarkdown: (node: JSONContent) => {
    const language = String(node.attrs?.language || "");
    const code = String(node.attrs?.code || "");
    return `\`\`\`${language}\n${code}\n\`\`\``;
  },

  addNodeView() {
    return VueNodeViewRenderer(asNodeViewComponent(EditorCodeBlockNode));
  },

  addCommands() {
    return {
      setCodeBlock:
        (attrs?: { language?: string }) =>
        ({ commands }) => {
          return commands.insertContent({
            type: this.name,
            attrs: { language: attrs?.language || null, code: "" },
          });
        },
      toggleCodeBlock:
        (attrs?: { language?: string }) =>
        ({ state, commands }) => {
          const { selection } = state;
          const node = selection.$from.parent;
          if (node.type.name === this.name) {
            // 切回段落时保留代码文本。
            return commands.insertContent({
              type: "paragraph",
              content: node.attrs.code
                ? [{ type: "text", text: node.attrs.code }]
                : [],
            });
          }
          return commands.insertContent({
            type: this.name,
            attrs: { language: attrs?.language || null, code: "" },
          });
        },
    };
  },

  addKeyboardShortcuts() {
    return {
      "Mod-Alt-c": () => this.editor.commands.toggleCodeBlock(),
    };
  },

  addInputRules() {
    return [];
  },
});
