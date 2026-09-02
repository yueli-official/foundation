import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

function read(path) {
  return readFileSync(resolve(packageRoot, path), "utf8");
}

test("markdown renderer exposes one interface for rich content and heading outlines", async () => {
  const { useMarkdown } = await import("../app/composables/useMarkdown.ts");
  const { renderWithToc } = useMarkdown();
  const result = renderWithToc(`# Markdown 渲染验收长文

## 重复标题
## 重复标题

==高亮文本==

> [!NOTE]
> 提示块

\`\`\`ts
const answer = 42
\`\`\`

$$
\\int_{-\\infty}^{\\infty} e^{-x^2} dx
$$

\`\`\`mermaid
graph TD
  A-->B
\`\`\`
`);

  assert.deepEqual(result.toc, [
    { id: "markdown-渲染验收长文", text: "Markdown 渲染验收长文", level: 1 },
    { id: "重复标题", text: "重复标题", level: 2 },
    { id: "重复标题-2", text: "重复标题", level: 2 },
  ]);
  assert.match(result.html, /<mark>高亮文本<\/mark>/);
  assert.match(result.html, /markdown-alert-note/);
  assert.match(result.html, /hljs language-ts/);
  assert.match(result.html, /class="math-block"/);
  assert.match(result.html, /class="language-mermaid"/);
});

test("shared article stylesheet covers rich markdown reading affordances", () => {
  const css = read("app/assets/css/article.css");

  assert.match(css, /\.content-prose :where\(table\)/);
  assert.match(css, /\.prose :where\(ul\.contains-task-list\)/);
  assert.match(css, /li:has\(> input\[type="checkbox"\]\)/);
  assert.match(css, /li > input\[type="checkbox"\]:first-child/);
  assert.match(css, /\.prose :where\(:not\(pre\) > code\)/);
  assert.match(css, /\.prose :where\(kbd\)/);
  assert.match(css, /\.prose :where\(mark\)/);
  assert.match(css, /highlight\.js\/styles\/stackoverflow-light\.css/);
  assert.match(css, /StackOverflow Dark/);
});

test("editor toolbar controls expose names and stay in one mobile row", () => {
  const toolbar = read("app/composables/useEditorToolbar.ts");
  const editor = read("app/components/ContentEditor.client.vue");
  const tooltipLabels = [
    ...toolbar.matchAll(/tooltip:\s*\{\s*text:\s*"([^"]+)"\s*\}/gu),
  ].map((match) => match[1]);

  assert.ok(tooltipLabels.length > 10);
  for (const label of new Set(tooltipLabels)) {
    assert.match(toolbar, new RegExp(`"aria-label": "${label}"`, "u"));
  }
  assert.match(editor, /flex-nowrap[^"\n]*overflow-x-auto/u);
  assert.doesNotMatch(editor, /flex-wrap items-center/u);
});

test("article media can switch from thumbnail to a named preview rendition", async () => {
  const { contentAssetRenditionURL } = await import(
    "../app/utils/contentMedia.ts"
  );
  assert.equal(
    contentAssetRenditionURL(
      "/media/31Pj0mXv7cfR5fdZIUvra?format=webp&name=thumbnail&v=1",
      "content",
    ),
    "/media/31Pj0mXv7cfR5fdZIUvra?format=webp&name=content&v=1",
  );
  assert.equal(
    contentAssetRenditionURL("https://external.example/image.png", "content"),
    "",
  );
  const prose = read("app/components/ContentProse.vue");
  assert.match(prose, /data-content-preview-url/);
  assert.match(prose, /<UModal/);
});
