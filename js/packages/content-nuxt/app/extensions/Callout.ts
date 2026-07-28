import { Node, mergeAttributes, type CommandProps } from '@tiptap/core'
import { VueNodeViewRenderer } from '@tiptap/vue-3'
import EditorCalloutNode from '../components/EditorCalloutNode.vue'

// GitHub-style callout / admonition blocks (> [!NOTE] …) (editor E6). Ported from
// the donor admin editor. Intercepts the blockquote markdown token to distinguish
// callouts from plain blockquotes; the reading side renders via marked-alert.
export type CalloutType = 'note' | 'tip' | 'important' | 'warning' | 'caution'

const CALLOUT_RE = /^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*/i

declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    callout: {
      setCallout: (attrs?: { type?: CalloutType }) => ReturnType
    }
  }
}

export const Callout = Node.create({
  name: 'callout',
  group: 'block',
  content: 'block+',
  defining: true,

  addAttributes() {
    return {
      type: { default: 'note' as CalloutType },
    }
  },

  parseHTML() {
    return [{
      tag: 'div[data-callout]',
      getAttrs: (el: string | HTMLElement) => ({
        type: (typeof el === 'string' ? 'note' : el.getAttribute('data-callout')) || 'note',
      }),
    }]
  },

  renderHTML({ HTMLAttributes }: { HTMLAttributes: Record<string, any> }) {
    return ['div', mergeAttributes({ 'data-callout': HTMLAttributes.type, class: `callout callout-${HTMLAttributes.type}` }, HTMLAttributes), 0]
  },

  addNodeView() {
    return VueNodeViewRenderer(EditorCalloutNode as any)
  },

  // Intercept the built-in blockquote token (same pattern as CodeBlockWithLang using 'code')
  // This avoids custom tokenizer registration issues — marked.js's blockquote tokenizer
  // reliably captures "> [!NOTE]" content, and we distinguish callouts from regular blockquotes
  // in parseMarkdown.
  markdownTokenName: 'blockquote',

  parseMarkdown: (token: any, helpers: any) => {
    const text = token.text || ''
    const match = text.match(CALLOUT_RE)

    if (!match) {
      // Regular blockquote — we must handle it here since we intercept the token
      const parseBlockChildren = helpers.parseBlockChildren ?? helpers.parseChildren
      return helpers.createNode('blockquote', {}, parseBlockChildren(token.tokens || []))
    }

    // It's a callout: > [!TYPE]
    const calloutType = match[1]!.toLowerCase() as CalloutType
    const parseBlockChildren = helpers.parseBlockChildren ?? helpers.parseChildren

    // Parse all inner tokens as-is first, then strip the [!TYPE] prefix
    // from the resulting ProseMirror JSON (safer than mutating marked tokens)
    const children = parseBlockChildren(token.tokens || []) as any[] | null

    if (children?.length) {
      const first = children[0]
      if (first.type === 'paragraph' && first.content?.length) {
        const firstText = first.content[0]
        if (firstText.type === 'text' && firstText.text) {
          const prefixMatch = firstText.text.match(CALLOUT_RE)
          if (prefixMatch) {
            firstText.text = firstText.text.slice(prefixMatch[0].length)
            if (!firstText.text) {
              first.content.shift()
              // Also strip any leading hardBreak left by the \n after [!TYPE]
              if (first.content.length && first.content[0].type === 'hardBreak') {
                first.content.shift()
              }
            }
          }
        }
        // Remove first paragraph if it's empty after stripping
        if (!first.content?.length) {
          children.shift()
        }
      }
    }

    const content = children?.length ? children : [{ type: 'paragraph' }]
    return helpers.createNode('callout', { type: calloutType }, content)
  },

  renderMarkdown: (node: any, h: any) => {
    const type = (node.attrs?.type || 'note').toUpperCase()
    if (!node.content) {
      return `> [!${type}]\n> `
    }
    const children = h.renderChildren(node.content)
    const lines = children.split('\n').map((line: string) => `> ${line}`).join('\n')
    return `> [!${type}]\n${lines}`
  },

  addCommands() {
    return {
      setCallout:
        (attrs?: { type?: CalloutType }) =>
        ({ commands }: CommandProps) => {
          return commands.insertContent({
            type: this.name,
            attrs: { type: attrs?.type ?? 'note' },
            content: [{ type: 'paragraph' }],
          })
        },
    }
  },
})
