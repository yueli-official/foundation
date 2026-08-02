import {
  Node,
  mergeAttributes,
  type CommandProps,
  type JSONContent,
  type MarkdownParseHelpers,
  type MarkdownRendererHelpers,
  type MarkdownToken,
} from "@tiptap/core";
import { VueNodeViewRenderer } from "@tiptap/vue-3";
import EditorCalloutNode from "../components/EditorCalloutNode.vue";
import { asNodeViewComponent } from "./nodeViewComponent";

// GitHub 风格提示块（> [!NOTE] …）接管 blockquote token，并在解析时区分
// 普通引用和提示块；阅读侧由 marked-alert 使用同一 Markdown 语义渲染。
export type CalloutType = "note" | "tip" | "important" | "warning" | "caution";

const CALLOUT_RE = /^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*/i;

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    callout: {
      setCallout: (attrs?: { type?: CalloutType }) => ReturnType;
    };
  }
}

export const Callout = Node.create({
  name: "callout",
  group: "block",
  content: "block+",
  defining: true,

  addAttributes() {
    return {
      type: { default: "note" as CalloutType },
    };
  },

  parseHTML() {
    return [
      {
        tag: "div[data-callout]",
        getAttrs: (el: string | HTMLElement) => ({
          type:
            (typeof el === "string"
              ? "note"
              : el.getAttribute("data-callout")) || "note",
        }),
      },
    ];
  },

  renderHTML({ HTMLAttributes }: { HTMLAttributes: Record<string, unknown> }) {
    return [
      "div",
      mergeAttributes(
        {
          "data-callout": HTMLAttributes.type,
          class: `callout callout-${HTMLAttributes.type}`,
        },
        HTMLAttributes,
      ),
      0,
    ];
  },

  addNodeView() {
    return VueNodeViewRenderer(asNodeViewComponent(EditorCalloutNode));
  },

  // 复用 marked.js 稳定的 blockquote tokenizer，再在 parseMarkdown 中区分提示块。
  markdownTokenName: "blockquote",

  parseMarkdown: (token: MarkdownToken, helpers: MarkdownParseHelpers) => {
    const text = token.text || "";
    const match = text.match(CALLOUT_RE);

    if (!match) {
      // 接管 token 后，普通引用也必须在这里还原。
      const parseBlockChildren =
        helpers.parseBlockChildren ?? helpers.parseChildren;
      return helpers.createNode(
        "blockquote",
        {},
        parseBlockChildren(token.tokens || []),
      );
    }

    const calloutType = match[1]!.toLowerCase() as CalloutType;
    const parseBlockChildren =
      helpers.parseBlockChildren ?? helpers.parseChildren;

    // 先解析内部 token，再从 ProseMirror JSON 移除前缀，避免修改 marked token。
    const children = parseBlockChildren(token.tokens || []);

    if (children?.length) {
      const first = children[0];
      if (first?.type === "paragraph" && first.content?.length) {
        const firstText = first.content[0];
        if (firstText?.type === "text" && firstText.text) {
          const prefixMatch = firstText.text.match(CALLOUT_RE);
          if (prefixMatch) {
            firstText.text = firstText.text.slice(prefixMatch[0].length);
            if (!firstText.text) {
              first.content.shift();
              // 同时移除 [!TYPE] 后换行留下的首个 hardBreak。
              if (
                first.content.length &&
                first.content[0]?.type === "hardBreak"
              ) {
                first.content.shift();
              }
            }
          }
        }
        // 前缀移除后首段为空则一并删除。
        if (!first.content?.length) {
          children.shift();
        }
      }
    }

    const content = children?.length ? children : [{ type: "paragraph" }];
    return helpers.createNode("callout", { type: calloutType }, content);
  },

  renderMarkdown: (node: JSONContent, helpers: MarkdownRendererHelpers) => {
    const type = String(node.attrs?.type || "note").toUpperCase();
    if (!node.content) {
      return `> [!${type}]\n> `;
    }
    const children = helpers.renderChildren(node.content);
    const lines = children
      .split("\n")
      .map((line: string) => `> ${line}`)
      .join("\n");
    return `> [!${type}]\n${lines}`;
  },

  addCommands() {
    return {
      setCallout:
        (attrs?: { type?: CalloutType }) =>
        ({ commands }: CommandProps) => {
          return commands.insertContent({
            type: this.name,
            attrs: { type: attrs?.type ?? "note" },
            content: [{ type: "paragraph" }],
          });
        },
    };
  },
});
