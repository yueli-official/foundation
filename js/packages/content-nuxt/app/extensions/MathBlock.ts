import { Node, mergeAttributes } from '@tiptap/core'
import { VueNodeViewRenderer } from '@tiptap/vue-3'
import EditorMathBlockNode from '../components/EditorMathBlockNode.vue'

// Block-level LaTeX math ($$…$$) rendered with KaTeX (editor E4). Ported from
// the donor admin editor. Inline math ($…$) is handled by @tiptap/extension-mathematics.
declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    mathBlock: {
      setMathBlock: (attrs?: { latex?: string }) => ReturnType
    }
  }
}

export const MathBlock = Node.create({
  name: 'blockMath',
  group: 'block',
  atom: true,
  selectable: true,
  draggable: true,

  addAttributes() {
    return {
      latex: { default: '' },
    }
  },

  parseHTML() {
    return [{ tag: 'div[data-type="block-math"]' }]
  },

  renderHTML({ HTMLAttributes }) {
    return ['div', mergeAttributes(HTMLAttributes, { 'data-type': 'block-math' })]
  },

  // @tiptap/markdown integration
  markdownTokenName: 'blockMath',
  markdownTokenizer: {
    name: 'blockMath',
    level: 'block' as const,
    start: '$$',
    tokenize(src: string) {
      const match = src.match(/^\$\$([\s\S]+?)\$\$/)
      if (!match) return undefined
      return {
        type: 'blockMath',
        raw: match[0],
        text: (match[1] ?? '').trim(),
      }
    },
  },
  parseMarkdown: (token: any, helpers: any) => {
    return helpers.createNode('blockMath', { latex: token.text || '' })
  },
  renderMarkdown: (node: any) => {
    const latex = node.attrs?.latex || ''
    return `$$\n${latex}\n$$`
  },

  addNodeView() {
    return VueNodeViewRenderer(EditorMathBlockNode as any)
  },

  addCommands() {
    return {
      setMathBlock:
        (attrs?: { latex?: string }) =>
        ({ commands }) => {
          return commands.insertContent({
            type: this.name,
            attrs: { latex: attrs?.latex ?? '' },
          })
        },
    }
  },

  addInputRules() {
    return []
  },
})
