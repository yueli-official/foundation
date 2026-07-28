import { Node, mergeAttributes } from '@tiptap/core'
import { VueNodeViewRenderer } from '@tiptap/vue-3'
import EditorMermaidBlockNode from '../components/EditorMermaidBlockNode.vue'

// Mermaid diagram block (```mermaid fences) with a live SVG preview (editor E5).
// Ported from the donor admin editor.
declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    mermaidBlock: {
      setMermaidBlock: (attrs?: { code?: string }) => ReturnType
    }
  }
}

export const MermaidBlock = Node.create({
  name: 'mermaidBlock',
  group: 'block',
  atom: true,
  selectable: true,
  draggable: true,

  addAttributes() {
    return {
      code: { default: '' },
    }
  },

  parseHTML() {
    return [{ tag: 'div[data-type="mermaid-block"]' }]
  },

  renderHTML({ HTMLAttributes }) {
    return ['div', mergeAttributes(HTMLAttributes, { 'data-type': 'mermaid-block' })]
  },

  // @tiptap/markdown integration
  markdownTokenName: 'mermaidBlock',
  markdownTokenizer: {
    name: 'mermaidBlock',
    level: 'block' as const,
    start: '```mermaid',
    tokenize(src: string) {
      const match = src.match(/^```mermaid\n([\s\S]+?)```/)
      if (!match) return undefined
      return {
        type: 'mermaidBlock',
        raw: match[0],
        text: (match[1] ?? '').trim(),
      }
    },
  },
  parseMarkdown: (token: any, helpers: any) => {
    return helpers.createNode('mermaidBlock', { code: token.text || '' })
  },
  renderMarkdown: (node: any) => {
    const code = node.attrs?.code || ''
    return `\`\`\`mermaid\n${code}\n\`\`\``
  },

  addNodeView() {
    return VueNodeViewRenderer(EditorMermaidBlockNode as any)
  },

  addCommands() {
    return {
      setMermaidBlock:
        (attrs?: { code?: string }) =>
        ({ commands }) => {
          return commands.insertContent({
            type: this.name,
            attrs: { code: attrs?.code ?? '' },
          })
        },
    }
  },

  addInputRules() {
    return []
  },
})
