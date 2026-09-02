import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const source = readFileSync(
  fileURLToPath(
    new URL(
      "../src/feedback/components/FeedbackToastRegion.client.vue",
      import.meta.url,
    ),
  ),
  "utf8",
);

describe("FeedbackToastRegion", () => {
  it("uses valid live-region elements and never intercepts while hidden", () => {
    expect(source).toContain('<div\n        v-for="notice in notices"');
    expect(source).not.toContain("<article");
    expect(source).toContain(
      '.yueli-toast-region[aria-hidden="true"] .yueli-toast-notice',
    );
    expect(source).toMatch(
      /\.yueli-toast-leave-active\s*\{\s*pointer-events: none;/u,
    );
  });
});
