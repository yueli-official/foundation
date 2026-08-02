import type {
  EditorToolbarItem,
  EditorSuggestionMenuItem,
  DropdownMenuItem,
} from "@nuxt/ui";
import type { JSONContent, Editor } from "@tiptap/vue-3";
import { mapEditorItems } from "@nuxt/ui/utils/editor";

// Toolbar / bubble / suggestion / drag-handle item config for the blog post
// editor. Ported from donor (Chinese labels, no i18n); the plugin toolbar,
// @mention menu, link popover and toolbar-overflow popover are dropped — the
// blog editor uses a horizontally-scrollable fixed toolbar and the built-in
// link handler.
export const useEditorToolbar = () => {
  const selectedNode = ref<{ node: JSONContent; pos: number }>();

  const toolbarItems = computed<EditorToolbarItem[][]>(() => [
    [
      {
        kind: "undo",
        icon: "i-tabler-arrow-back-up",
        tooltip: { text: "撤销" },
      },
      {
        kind: "redo",
        icon: "i-tabler-arrow-forward-up",
        tooltip: { text: "重做" },
      },
    ],
    [
      {
        icon: "i-tabler-heading",
        tooltip: { text: "标题" },
        content: { align: "start" },
        items: [
          { kind: "heading", level: 1, icon: "i-tabler-h-1", label: "标题 1" },
          { kind: "heading", level: 2, icon: "i-tabler-h-2", label: "标题 2" },
          { kind: "heading", level: 3, icon: "i-tabler-h-3", label: "标题 3" },
          { kind: "heading", level: 4, icon: "i-tabler-h-4", label: "标题 4" },
        ],
      },
      {
        icon: "i-tabler-list",
        tooltip: { text: "列表" },
        content: { align: "start" },
        items: [
          { kind: "bulletList", icon: "i-tabler-list", label: "无序列表" },
          {
            kind: "orderedList",
            icon: "i-tabler-list-numbers",
            label: "有序列表",
          },
        ],
      },
      {
        kind: "blockquote",
        icon: "i-tabler-blockquote",
        tooltip: { text: "引用" },
      },
      {
        kind: "codeBlock",
        icon: "i-tabler-code-dots",
        tooltip: { text: "代码块" },
      },
    ],
    [
      {
        kind: "mark",
        mark: "bold",
        icon: "i-tabler-bold",
        tooltip: { text: "粗体" },
      },
      {
        kind: "mark",
        mark: "italic",
        icon: "i-tabler-italic",
        tooltip: { text: "斜体" },
      },
      {
        kind: "mark",
        mark: "underline",
        icon: "i-tabler-underline",
        tooltip: { text: "下划线" },
      },
      {
        kind: "mark",
        mark: "strike",
        icon: "i-tabler-strikethrough",
        tooltip: { text: "删除线" },
      },
      {
        kind: "mark",
        mark: "code",
        icon: "i-tabler-code",
        tooltip: { text: "行内代码" },
      },
    ],
    [
      { kind: "link", icon: "i-tabler-link", tooltip: { text: "链接" } },
      { kind: "image", icon: "i-tabler-photo", tooltip: { text: "图片" } },
      {
        kind: "horizontalRule",
        icon: "i-tabler-separator",
        tooltip: { text: "分隔线" },
      },
    ],
    [
      {
        icon: "i-tabler-alert-circle",
        tooltip: { text: "告示块" },
        content: { align: "start" },
        items: [
          { kind: "callout-note", icon: "i-tabler-info-circle", label: "提示" },
          { kind: "callout-tip", icon: "i-tabler-bulb", label: "建议" },
          {
            kind: "callout-important",
            icon: "i-tabler-alert-circle",
            label: "重要",
          },
          {
            kind: "callout-warning",
            icon: "i-tabler-alert-triangle",
            label: "警告",
          },
          { kind: "callout-caution", icon: "i-tabler-flame", label: "危险" },
        ],
      },
      {
        icon: "i-tabler-math",
        tooltip: { text: "公式" },
        content: { align: "start" },
        items: [
          { kind: "math-inline", icon: "i-tabler-math", label: "行内公式" },
          {
            kind: "math-block",
            icon: "i-tabler-math-function",
            label: "公式块",
          },
        ],
      },
      {
        kind: "mermaid-block",
        icon: "i-tabler-chart-dots-3",
        tooltip: { text: "Mermaid 图表" },
      },
    ],
    [
      {
        icon: "i-tabler-align-justified",
        tooltip: { text: "对齐" },
        content: { align: "end" },
        items: [
          {
            kind: "textAlign",
            align: "left",
            icon: "i-tabler-align-left",
            label: "左对齐",
          },
          {
            kind: "textAlign",
            align: "center",
            icon: "i-tabler-align-center",
            label: "居中",
          },
          {
            kind: "textAlign",
            align: "right",
            icon: "i-tabler-align-right",
            label: "右对齐",
          },
          {
            kind: "textAlign",
            align: "justify",
            icon: "i-tabler-align-justified",
            label: "两端对齐",
          },
        ],
      },
    ],
    [
      {
        kind: "clearFormatting",
        icon: "i-tabler-clear-formatting",
        tooltip: { text: "清除格式" },
      },
    ],
  ]);

  const bubbleItems = computed<EditorToolbarItem[][]>(() => [
    [
      {
        label: "转换为",
        trailingIcon: "i-tabler-chevron-down",
        activeColor: "neutral",
        activeVariant: "ghost",
        content: { align: "start" },
        ui: { label: "text-xs" },
        items: [
          { type: "label", label: "转换为" },
          { kind: "paragraph", label: "正文", icon: "i-tabler-text-size" },
          { kind: "heading", level: 1, icon: "i-tabler-h-1", label: "标题 1" },
          { kind: "heading", level: 2, icon: "i-tabler-h-2", label: "标题 2" },
          { kind: "heading", level: 3, icon: "i-tabler-h-3", label: "标题 3" },
          { kind: "bulletList", label: "无序列表", icon: "i-tabler-list" },
          {
            kind: "orderedList",
            label: "有序列表",
            icon: "i-tabler-list-numbers",
          },
          { kind: "blockquote", label: "引用", icon: "i-tabler-blockquote" },
          { kind: "codeBlock", label: "代码块", icon: "i-tabler-code-dots" },
        ],
      },
    ],
    [
      { kind: "mark", mark: "bold", icon: "i-tabler-bold" },
      { kind: "mark", mark: "italic", icon: "i-tabler-italic" },
      { kind: "mark", mark: "underline", icon: "i-tabler-underline" },
      { kind: "mark", mark: "strike", icon: "i-tabler-strikethrough" },
      { kind: "mark", mark: "code", icon: "i-tabler-code" },
    ],
    [
      { kind: "link", icon: "i-tabler-link" },
      { kind: "clearFormatting", icon: "i-tabler-clear-formatting" },
    ],
  ]);

  const suggestionItems = computed<EditorSuggestionMenuItem[][]>(() => [
    [
      { type: "label", label: "样式" },
      { kind: "paragraph", label: "正文", icon: "i-tabler-text-size" },
      { kind: "heading", level: 1, label: "标题 1", icon: "i-tabler-h-1" },
      { kind: "heading", level: 2, label: "标题 2", icon: "i-tabler-h-2" },
      { kind: "heading", level: 3, label: "标题 3", icon: "i-tabler-h-3" },
      { kind: "heading", level: 4, label: "标题 4", icon: "i-tabler-h-4" },
      { kind: "bulletList", label: "无序列表", icon: "i-tabler-list" },
      { kind: "orderedList", label: "有序列表", icon: "i-tabler-list-numbers" },
      { kind: "blockquote", label: "引用", icon: "i-tabler-blockquote" },
      { kind: "codeBlock", label: "代码块", icon: "i-tabler-code-dots" },
    ],
    [
      { type: "label", label: "插入" },
      { kind: "emoji", label: "Emoji", icon: "i-tabler-mood-smile" },
      { kind: "image", label: "图片", icon: "i-tabler-photo" },
      {
        kind: "horizontalRule",
        label: "分隔线",
        icon: "i-tabler-separator-horizontal",
      },
      { kind: "callout-note", label: "告示块", icon: "i-tabler-alert-circle" },
      { kind: "math-inline", label: "行内公式", icon: "i-tabler-math" },
      { kind: "math-block", label: "公式块", icon: "i-tabler-math-function" },
      {
        kind: "mermaid-block",
        label: "Mermaid 图表",
        icon: "i-tabler-chart-dots-3",
      },
    ],
  ]);

  const imageBubbleItems = computed<EditorToolbarItem[][]>(() => [
    [
      {
        kind: "download-image",
        icon: "i-tabler-download",
        tooltip: { text: "下载图片" },
      },
      {
        kind: "remove-image",
        icon: "i-tabler-trash",
        tooltip: { text: "删除图片" },
      },
    ],
  ]);

  const dragHandleItems = (editor: Editor): DropdownMenuItem[][] => {
    if (!selectedNode.value?.node?.type) return [];
    return mapEditorItems(
      editor,
      [
        [
          { type: "label", label: String(selectedNode.value.node.type) },
          {
            label: "转换为",
            icon: "i-tabler-transform",
            children: [
              { kind: "paragraph", label: "正文", icon: "i-tabler-text-size" },
              {
                kind: "heading",
                level: 1,
                label: "标题 1",
                icon: "i-tabler-h-1",
              },
              {
                kind: "heading",
                level: 2,
                label: "标题 2",
                icon: "i-tabler-h-2",
              },
              {
                kind: "heading",
                level: 3,
                label: "标题 3",
                icon: "i-tabler-h-3",
              },
              { kind: "bulletList", label: "无序列表", icon: "i-tabler-list" },
              {
                kind: "orderedList",
                label: "有序列表",
                icon: "i-tabler-list-numbers",
              },
              {
                kind: "blockquote",
                label: "引用",
                icon: "i-tabler-blockquote",
              },
              {
                kind: "codeBlock",
                label: "代码块",
                icon: "i-tabler-code-dots",
              },
            ],
          },
          {
            kind: "clearFormatting",
            pos: selectedNode.value?.pos,
            label: "清除格式",
            icon: "i-tabler-clear-formatting",
          },
        ],
        [
          {
            kind: "duplicate",
            pos: selectedNode.value?.pos,
            label: "复制块",
            icon: "i-tabler-copy",
          },
          {
            label: "复制到剪贴板",
            icon: "i-tabler-clipboard",
            onSelect: async () => {
              if (!selectedNode.value) return;
              const node = editor.state.doc.nodeAt(selectedNode.value.pos);
              if (node) await navigator.clipboard.writeText(node.textContent);
            },
          },
        ],
        [
          {
            kind: "moveUp",
            pos: selectedNode.value?.pos,
            label: "上移",
            icon: "i-tabler-arrow-up",
          },
          {
            kind: "moveDown",
            pos: selectedNode.value?.pos,
            label: "下移",
            icon: "i-tabler-arrow-down",
          },
        ],
        [
          {
            kind: "delete",
            pos: selectedNode.value?.pos,
            label: "删除",
            icon: "i-tabler-trash",
          },
        ],
      ],
      {},
    ) as DropdownMenuItem[][];
  };

  return {
    toolbarItems,
    bubbleItems,
    suggestionItems,
    imageBubbleItems,
    selectedNode,
    dragHandleItems,
  };
};
