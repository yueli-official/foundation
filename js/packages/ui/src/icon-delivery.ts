import { FOUNDATION_TABLER_ICONS } from "./theme/index.js";
import { ADMIN_TABLER_ICON_OPTIONS } from "./admin/icons";
import { PUBLIC_COMMENT_TABLER_ICONS } from "./comments/icons";

const tablerCssName = /^i-tabler-([a-z0-9]+(?:-[a-z0-9]+)*)$/;
const tablerIconifyName = /^tabler:([a-z0-9]+(?:-[a-z0-9]+)*)$/;

export function normalizeTablerIconName(value: string): string {
  const candidate = value.trim();
  const match =
    tablerCssName.exec(candidate) ?? tablerIconifyName.exec(candidate);
  if (!match) {
    throw new Error(
      `Expected a finite Tabler icon name (i-tabler-* or tabler:*), received ${JSON.stringify(value)}`,
    );
  }
  return `tabler:${match[1]}`;
}

export function createTablerIconDelivery(dynamicIcons: readonly string[] = []) {
  const icons = Array.from(
    new Set(
      [
        ...Object.values(FOUNDATION_TABLER_ICONS),
        ...ADMIN_TABLER_ICON_OPTIONS.map((icon) => icon.value),
        ...PUBLIC_COMMENT_TABLER_ICONS,
        ...dynamicIcons,
      ].map(normalizeTablerIconName),
    ),
  ).sort();

  return {
    provider: "server" as const,
    fallbackToApi: false as const,
    serverBundle: { collections: ["tabler"] },
    clientBundle: {
      icons,
      scan: {
        globInclude: [
          "app/**/*.{vue,js,mjs,ts,jsx,tsx}",
          "components/**/*.{vue,js,mjs,ts,jsx,tsx}",
          "composables/**/*.{js,mjs,ts}",
          "layouts/**/*.{vue,js,mjs,ts,jsx,tsx}",
          "pages/**/*.{vue,js,mjs,ts,jsx,tsx}",
          "plugins/**/*.{js,mjs,ts}",
          "node_modules/@yueli/**/*.{vue,js,mjs,ts,jsx,tsx}",
        ],
        globExclude: [
          "test/**",
          "tests/**",
          "coverage/**",
          "dist/**",
          ".nuxt/**",
          ".output/**",
          ".*",
        ],
      },
      sizeLimitKb: 256,
    },
  };
}
