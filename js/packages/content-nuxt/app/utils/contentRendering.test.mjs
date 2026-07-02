import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../../../..')

function read(path) {
  return readFileSync(resolve(root, path), 'utf8')
}

test('docs markdown rendering fixture covers editor and reader feature blocks', () => {
  const fixture = read('scripts/devkit/fixtures/docs_markdown_rendering_checklist.md')

  assert.match(fixture, /# Markdown 渲染验收长文/)
  assert.match(fixture, /\*\*加粗文本\*\*/)
  assert.match(fixture, /~~删除线文本~~/)
  assert.match(fixture, /==高亮文本==/)
  assert.match(fixture, /<kbd>Ctrl<\/kbd>/)
  assert.match(fixture, /- \[x\] 普通段落可以渲染/)
  assert.match(fixture, /\| 模块 \| 编辑器输入 \| 前台渲染 \| 检查重点 \|/)
  assert.match(fixture, /> \[!NOTE\]/)
  assert.match(fixture, /```ts/)
  assert.match(fixture, /```vue/)
  assert.match(fixture, /\$\$\n\\int_\{-\\infty\}/)
  assert.match(fixture, /```mermaid/)
  assert.match(fixture, /!\[文档集封面示例\]/)
})

test('shared article stylesheet covers rich markdown reading affordances', () => {
  const css = read('packages/js/content/app/assets/css/article.css')

  assert.match(css, /\.content-prose :where\(table\)/)
  assert.match(css, /\.prose :where\(ul\.contains-task-list\)/)
  assert.match(css, /li:has\(> input\[type="checkbox"\]\)/)
  assert.match(css, /li > input\[type="checkbox"\]:first-child/)
  assert.match(css, /\.prose :where\(:not\(pre\) > code\)/)
  assert.match(css, /\.prose :where\(kbd\)/)
  assert.match(css, /\.prose :where\(mark\)/)
  assert.match(css, /highlight\.js\/styles\/stackoverflow-light\.css/)
  assert.match(css, /StackOverflow Dark/)
})
