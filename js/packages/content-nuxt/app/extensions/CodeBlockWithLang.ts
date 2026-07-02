import { Node, mergeAttributes } from '@tiptap/core'
import { VueNodeViewRenderer } from '@tiptap/vue-3'
import EditorCodeBlockNode from '../components/EditorCodeBlockNode.vue'

// Code block with a language picker + highlight.js preview (editor E3). Replaces
// the starter-kit codeBlock (same node name) so the markdown round-trips through
// ```lang fences. Ported from the donor admin editor.
export const CodeBlockWithLang = Node.create({
  name: 'codeBlock',
  group: 'block',
  atom: true,
  selectable: true,
  draggable: true,

  addAttributes() {
    return {
      language: { default: null },
      code: { default: '' },
    }
  },

  parseHTML() {
    return [
      {
        tag: 'pre',
        preserveWhitespace: 'full' as const,
        getAttrs: (el: string | HTMLElement) => {
          if (typeof el === 'string') return {}
          const code = el.querySelector('code')
          if (!code) return { code: typeof el === 'string' ? '' : el.textContent || '' }
          const cls = [...code.classList].find(c => c.startsWith('language-'))
          return {
            language: cls ? cls.replace('language-', '') : null,
            code: code.textContent || '',
          }
        },
      },
    ]
  },

  renderHTML({ node, HTMLAttributes }) {
    const lang = node.attrs.language
    const codeAttrs = lang ? { class: `language-${lang}` } : {}
    return ['pre', mergeAttributes(HTMLAttributes), ['code', codeAttrs, node.attrs.code || '']]
  },

  // @tiptap/markdown integration
  markdownTokenName: 'code',
  parseMarkdown: (token: any, helpers: any) => {
    if (token.raw?.startsWith('```') === false && token.raw?.startsWith('~~~') === false && token.codeBlockStyle !== 'indented') {
      return []
    }
    return helpers.createNode(
      'codeBlock',
      { language: token.lang || null, code: token.text || '' },
    )
  },
  renderMarkdown: (node: any) => {
    const language = node.attrs?.language || ''
    const code = node.attrs?.code || ''
    return `\`\`\`${language}\n${code}\n\`\`\``
  },

  addNodeView() {
    return VueNodeViewRenderer(EditorCodeBlockNode as any)
  },

  addCommands() {
    return {
      setCodeBlock:
        (attrs?: { language?: string }) =>
        ({ commands }) => {
          return commands.insertContent({
            type: this.name,
            attrs: { language: attrs?.language || null, code: '' },
          })
        },
      toggleCodeBlock:
        (attrs?: { language?: string }) =>
        ({ state, commands }) => {
          const { selection } = state
          const node = selection.$from.parent
          if (node.type.name === this.name) {
            // Convert code block to paragraph with its code content
            return commands.insertContent({
              type: 'paragraph',
              content: node.attrs.code ? [{ type: 'text', text: node.attrs.code }] : [],
            })
          }
          return commands.insertContent({
            type: this.name,
            attrs: { language: attrs?.language || null, code: '' },
          })
        },
    }
  },

  addKeyboardShortcuts() {
    return {
      'Mod-Alt-c': () => this.editor.commands.toggleCodeBlock(),
    }
  },

  addInputRules() {
    return []
  },
})
