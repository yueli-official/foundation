import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { FOUNDATION_TABLER_ICONS } from "../src/theme/index.js";
import {
  createTablerIconDelivery,
  normalizeTablerIconName,
} from "../src/icon-delivery";

describe("shared Tabler delivery", () => {
  it("bundles Foundation and consumer-owned dynamic icons without runtime fetching", () => {
    const delivery = createTablerIconDelivery([
      "i-tabler-palette",
      "tabler:photo",
      "i-tabler-palette",
    ]);

    expect(delivery.provider).toBe("none");
    expect(delivery.fallbackToApi).toBe(false);
    expect(delivery.serverBundle).toEqual({ collections: ["tabler"] });
    expect(delivery.clientBundle.icons).toContain("tabler:palette");
    expect(delivery.clientBundle.icons).toContain("tabler:photo");
    expect(delivery.clientBundle.icons).toContain("tabler:arrow-left");
    expect(new Set(delivery.clientBundle.icons).size).toBe(
      delivery.clientBundle.icons.length,
    );
    expect(delivery.clientBundle.scan).toMatchObject({
      globInclude: expect.arrayContaining([
        "app/**/*.{vue,js,mjs,ts,jsx,tsx}",
        "node_modules/@yueli/**/*.{vue,js,mjs,ts,jsx,tsx}",
      ]),
    });

    for (const icon of Object.values(FOUNDATION_TABLER_ICONS)) {
      expect(delivery.clientBundle.icons).toContain(
        normalizeTablerIconName(icon),
      );
    }
  });

  it("rejects icons outside the finite Tabler contract", () => {
    expect(() => normalizeTablerIconName("i-lucide-house")).toThrow(
      /Tabler/,
    );
    expect(() => normalizeTablerIconName("i-tabler-../house")).toThrow(
      /Tabler/,
    );
  });

  it("wires the delivery contract through the Nuxt module dependency", () => {
    const moduleSource = readFileSync(
      new URL("../src/module.ts", import.meta.url),
      "utf8",
    );

    expect(moduleSource).toContain("moduleDependencies(nuxt)");
    expect(moduleSource).toContain('"@nuxt/icon"');
    expect(moduleSource).toContain("createTablerIconDelivery");
  });
});
