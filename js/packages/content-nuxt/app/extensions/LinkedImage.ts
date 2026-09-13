import {
  Node,
  mergeAttributes,
  type JSONContent,
  type MarkdownParseHelpers,
  type MarkdownToken,
} from "@tiptap/core";

type LinkedImageToken = MarkdownToken & {
  src?: string;
  alt?: string;
  title?: string | null;
  href?: string;
  linkTitle?: string | null;
};

const linkedImagePattern =
  /^\[!\[([^\]]*)\]\(([^\s)]+)(?:\s+["']([^"']*)["'])?\)\]\(([^\s)]+)(?:\s+["']([^"']*)["'])?\)/;

function markdownTitle(value?: string | null) {
  if (!value) return "";
  const escaped = value.replaceAll("\\", "\\\\").replaceAll('"', '\\"');
  return ` "${escaped}"`;
}

function markdownAlt(value?: string | null) {
  return (value || "").replaceAll("]", "\\]");
}

// @tiptap/markdown applies link marks only to text nodes, so a Markdown image
// nested inside a link silently loses the destination during rich-editor
// round trips. Preserve that syntax as one inline atom while keeping regular
// images on Nuxt UI's standard Image extension.
export const LinkedImage = Node.create({
  name: "linkedImage",
  inline: true,
  group: "inline",
  atom: true,
  selectable: true,
  draggable: true,

  addAttributes() {
    return {
      src: { default: null },
      alt: { default: "" },
      title: { default: null },
      href: { default: null },
      linkTitle: { default: null },
    };
  },

  parseHTML() {
    return [
      {
        tag: "span[data-linked-image]",
        getAttrs: (element: string | HTMLElement) => {
          if (typeof element === "string") return {};
          const image = element.querySelector("img");
          return {
            src: image?.getAttribute("src") || "",
            alt: image?.getAttribute("alt") || "",
            title: image?.getAttribute("title"),
            href: element.getAttribute("data-href") || "",
            linkTitle: element.getAttribute("data-link-title"),
          };
        },
      },
    ];
  },

  renderHTML({ HTMLAttributes }) {
    const { src, alt, title, href, linkTitle, ...rest } = HTMLAttributes;
    return [
      "span",
      mergeAttributes(rest, {
        "data-linked-image": "",
        "data-href": href || "",
        "data-link-title": linkTitle || undefined,
      }),
      ["img", { src, alt, title }],
    ];
  },

  markdownTokenName: "linkedImage",
  markdownTokenizer: {
    name: "linkedImage",
    level: "inline" as const,
    start: "[![",
    tokenize(src: string) {
      const match = linkedImagePattern.exec(src);
      if (!match) return undefined;
      return {
        type: "linkedImage",
        raw: match[0],
        alt: match[1] || "",
        src: match[2] || "",
        title: match[3] || null,
        href: match[4] || "",
        linkTitle: match[5] || null,
      };
    },
  },
  parseMarkdown: (token: MarkdownToken, helpers: MarkdownParseHelpers) => {
    const linked = token as LinkedImageToken;
    return helpers.createNode("linkedImage", {
      src: linked.src || "",
      alt: linked.alt || "",
      title: linked.title || null,
      href: linked.href || "",
      linkTitle: linked.linkTitle || null,
    });
  },
  renderMarkdown: (node: JSONContent) => {
    const attrs = node.attrs || {};
    const image = `![${markdownAlt(String(attrs.alt || ""))}](${String(attrs.src || "")}${markdownTitle(attrs.title as string | null)})`;
    return `[${image}](${String(attrs.href || "")}${markdownTitle(attrs.linkTitle as string | null)})`;
  },
});
