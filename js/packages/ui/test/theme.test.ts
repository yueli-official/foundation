import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  createUiPreset,
  DEFAULT_UI_THEME,
  FOUNDATION_TABLER_ICONS,
  foundationUiPreset,
} from "../src/theme/index.js";

describe("public theme contract", () => {
  it("builds a complete provider-neutral Nuxt UI preset", () => {
    const preset = createUiPreset({ primary: "emerald" });

    expect(preset.ui.colors).toEqual({ primary: "emerald", neutral: "stone" });
    expect(preset.ui.card).toEqual(foundationUiPreset.card);
    expect(preset.ui.icons).toBe(FOUNDATION_TABLER_ICONS);
    expect(DEFAULT_UI_THEME).toEqual({ primary: "blue", neutral: "stone" });
  });

  it("supports local colors, chrome and icon overrides without losing defaults", () => {
    const preset = createUiPreset(
      { primary: "brand", neutral: "slate" },
      { cardRoot: "rounded-xl", icons: { search: "i-custom-search" } },
    );

    expect(preset.ui.colors).toEqual({ primary: "brand", neutral: "slate" });
    expect(preset.ui.card.slots.root).toBe("rounded-xl");
    expect(preset.ui.icons.search).toBe("i-custom-search");
    expect(preset.ui.icons.close).toBe("i-tabler-x");
  });

  it("keeps one bundled icon family", () => {
    expect(
      Object.values(FOUNDATION_TABLER_ICONS).every((icon) =>
        icon.startsWith("i-tabler-"),
      ),
    ).toBe(true);
  });

  it("ships semantic light, dark, focus, motion and surface roles", () => {
    const css = readFileSync(
      resolve(import.meta.dirname, "../src/theme.css"),
      "utf8",
    );

    for (const token of [
      "--yueli-surface-page",
      "--yueli-surface-region",
      "--yueli-surface-card",
      "--yueli-surface-inset",
      "--yueli-border-focus",
      "--yueli-accent-border",
      "--yueli-radius-surface",
      "--yueli-motion-standard",
    ]) {
      expect(css).toContain(token);
    }
    expect(css).toContain(":focus-visible");
    expect(css).toContain(".dark");
    expect(css).toContain(".yueli-interactive");
    expect(css).not.toContain("--platform-");
  });
});
