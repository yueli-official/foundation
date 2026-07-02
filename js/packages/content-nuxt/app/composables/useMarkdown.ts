import { Marked, type Tokens } from 'marked'
import { gitHubEmojis, shortcodeToEmoji } from '@tiptap/extension-emoji'
import hljs from 'highlight.js'
import katex from 'katex'
import markedAlert from 'marked-alert'

// Reading-side markdown → HTML pipeline (editor roadmap E3-E6 reading ends):
// highlight.js code highlighting (E3), KaTeX math (E4), mermaid passthrough for
// client-side rendering in BlogProse (E5), and GitHub-style callouts via
// marked-alert (E6). marked is deterministic, so SSR and client produce the same
// string (no hydration mismatch). Ported from the donor reading renderer.
//
// IMPORTANT: use an ISOLATED `new Marked()` instance, NOT the global `marked`
// singleton. @nuxt/ui's editor uses @tiptap/markdown, which shares the global
// marked singleton — calling `marked.use(...)` on it would leak our math/code
// extensions into the editor's parser and crash it ("KaTeX can only parse string
// typed expression"). A private instance keeps the two parsers independent.
//
// NOTE: content is authored through the JWT-gated editor and rendered as-is —
// sanitize (DOMPurify / isomorphic) before opening multi-author authoring.

const md = new Marked()

function escapeAttr(value: string): string {
  return value.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;')
}

// ── Heading anchors + TOC capture ─────────────────────────────────────────
// Each render assigns a stable id to every heading (so a TOC / deep links can
// target them) and records the heading list. CJK is preserved in the slug.
// `captured`/`seenIds` are reset at the start of each parse() — md.parse is
// synchronous, so there is no cross-render interleave even under SSR.
export interface TocEntry { id: string, text: string, level: number }

let captured: TocEntry[] = []
const seenIds = new Set<string>()

function headingSlug(text: string): string {
  let base = text.toLowerCase().trim()
    .replace(/\s+/g, '-')
    .replace(/[^\p{L}\p{N}-]+/gu, '') // keep unicode letters/digits (incl. CJK) + hyphen
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '')
  if (!base) base = 'section'
  let id = base
  let n = 2
  while (seenIds.has(id)) { id = `${base}-${n++}` }
  seenIds.add(id)
  return id
}

md.use({
  renderer: {
    heading(token: Tokens.Heading) {
      const inner = this.parser.parseInline(token.tokens)
      const text = inner.replace(/<[^>]+>/g, '').trim()
      const id = headingSlug(text)
      captured.push({ id, text, level: token.depth })
      return `<h${token.depth} id="${id}">${inner}</h${token.depth}>\n`
    },
  },
})

// ── Emoji shortcode renderer ──────────────────────────────────────────────
// TipTap's emoji extension stores authored emoji as GitHub-style shortcodes
// (for example :rocket:). Reading-side markdown must render the same semantic
// content instead of leaking raw shortcodes into previews and public posts.
md.use({
  extensions: [{
    name: 'emojiShortcode',
    level: 'inline' as const,
    start(src: string) { return src.indexOf(':') },
    tokenizer(src: string) {
      const match = src.match(/^:([a-z0-9_+-]+):/i)
      if (!match) return
      const shortcode = match[1] ?? ''
      const item = shortcodeToEmoji(shortcode, gitHubEmojis)
      if (!item?.emoji) return
      return { type: 'emojiShortcode', raw: match[0], text: item.emoji, shortcode }
    },
    renderer(token: Tokens.Generic) {
      const shortcode = escapeAttr(String(token.shortcode ?? 'emoji'))
      return `<span class="content-emoji" role="img" aria-label=":${shortcode}:" title=":${shortcode}:">${token.text}</span>`
    },
  }],
})

// ── Inline highlight (`==mark==`) ─────────────────────────────────────────
// The rich editor may not expose every Markdown token as a toolbar button, but
// authored Markdown should still render common GFM-adjacent notation cleanly.
md.use({
  extensions: [{
    name: 'markHighlight',
    level: 'inline' as const,
    start(src: string) { return src.indexOf('==') },
    tokenizer(src: string) {
      const match = src.match(/^==(?=\S)([\s\S]*?\S)==/)
      if (!match) return
      return { type: 'markHighlight', raw: match[0], tokens: this.lexer.inlineTokens(match[1] ?? '') }
    },
    renderer(token: Tokens.Generic) {
      return `<mark>${this.parser.parseInline(token.tokens ?? [])}</mark>`
    },
  }],
})

// ── KaTeX math extension ─────────────────────────────────────────────────
md.use({
  extensions: [{
    name: 'blockMath',
    level: 'block' as const,
    start(src: string) { return src.indexOf('$$') },
    tokenizer(src: string) {
      const match = src.match(/^\$\$([\s\S]+?)\$\$/)
      if (match) return { type: 'blockMath', raw: match[0], text: (match[1] ?? '').trim() }
    },
    renderer(token: Tokens.Generic) {
      return `<div class="math-block">${katex.renderToString(token.text, { displayMode: true, throwOnError: false })}</div>`
    },
  }, {
    name: 'inlineMath',
    level: 'inline' as const,
    start(src: string) { return src.indexOf('$') },
    tokenizer(src: string) {
      const match = src.match(/^\$([^\s$](?:[^$]*[^\s$])?)\$/)
      if (match) return { type: 'inlineMath', raw: match[0], text: match[1] }
    },
    renderer(token: Tokens.Generic) {
      return katex.renderToString(token.text, { throwOnError: false })
    },
  }],
})

// ── GitHub-style callouts ────────────────────────────────────────────────
md.use(markedAlert())

// ── Code renderer: highlight.js + mermaid passthrough ────────────────────
md.use({
  renderer: {
    code({ text, lang }: { text: string, lang?: string }) {
      if (lang === 'mermaid') {
        return `<pre><code class="language-mermaid">${text}</code></pre>`
      }
      const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext'
      const highlighted = hljs.highlight(text, { language }).value
      return `<pre><code class="hljs language-${language}">${highlighted}</code></pre>`
    },
  },
})

// TipTap's blockquote serialization can insert a blank line between `> [!TYPE]`
// and `> content`, which breaks marked-alert parsing. Merge them back.
function normalizeMarkdown(src: string): string {
  return src.replace(
    /^([ \t]*> \[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\])\s*\n\s*\n((?:[ \t]*> .*(?:\n|$))+)/gm,
    '$1\n$3',
  )
}

function parse(input: string): { html: string, toc: TocEntry[] } {
  captured = []
  seenIds.clear()
  const html = md.parse(normalizeMarkdown(input ?? ''), { async: false }) as string
  return { html, toc: captured.slice() }
}

export function useMarkdown() {
  // render returns just the HTML (heading ids included) — the reading body.
  function render(input: string): string {
    return parse(input).html
  }
  // renderWithToc also returns the heading outline (for a table of contents).
  function renderWithToc(input: string): { html: string, toc: TocEntry[] } {
    return parse(input)
  }
  return { render, renderWithToc }
}
