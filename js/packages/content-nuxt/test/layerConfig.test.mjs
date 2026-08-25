import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

test("content layer owns the local KaTeX stylesheet filesystem boundary", () => {
  const config = readFileSync(resolve(root, "nuxt.config.ts"), "utf8");
  assert.match(config, /require\.resolve\("katex\/dist\/katex\.min\.css"\)/);
  assert.match(config, /realpathSync/);
  assert.match(config, /fs:\s*\{\s*allow:\s*\[katexStylesRoot\]\s*\}/);
});
