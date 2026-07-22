export const DEFAULT_UI_THEME = Object.freeze({
  primary: "blue",
  neutral: "stone",
});

/**
 * Framework icon contract shared by every consumer. It deliberately uses one
 * Iconify family so applications can bundle a predictable offline icon set.
 */
export const FOUNDATION_TABLER_ICONS = Object.freeze({
  arrowLeft: "i-tabler-arrow-left",
  arrowRight: "i-tabler-arrow-right",
  check: "i-tabler-check",
  chevronDoubleLeft: "i-tabler-chevrons-left",
  chevronDoubleRight: "i-tabler-chevrons-right",
  chevronDown: "i-tabler-chevron-down",
  chevronLeft: "i-tabler-chevron-left",
  chevronRight: "i-tabler-chevron-right",
  chevronUp: "i-tabler-chevron-up",
  close: "i-tabler-x",
  ellipsis: "i-tabler-dots",
  external: "i-tabler-external-link",
  loading: "i-tabler-loader-2",
  minus: "i-tabler-minus",
  plus: "i-tabler-plus",
  search: "i-tabler-search",
  light: "i-tabler-sun",
  dark: "i-tabler-moon",
  system: "i-tabler-device-desktop",
});

export const foundationUiPreset = Object.freeze({
  colors: Object.freeze({ neutral: DEFAULT_UI_THEME.neutral }),
  card: Object.freeze({
    slots: Object.freeze({
      root: "rounded-2xl shadow-[var(--shadow-soft)]",
    }),
  }),
  toast: Object.freeze({
    slots: Object.freeze({
      title: "line-clamp-1",
      description: "line-clamp-2",
    }),
  }),
});

/**
 * Builds a Nuxt `defineAppConfig` payload without knowing application names,
 * routes, locale catalogs or product palettes.
 *
 * @param {{ primary: string, neutral?: string }} [theme]
 * @param {{ icons?: Partial<typeof FOUNDATION_TABLER_ICONS>, cardRoot?: string, toastTitle?: string, toastDescription?: string }} [options]
 */
export function createUiPreset(theme = DEFAULT_UI_THEME, options = {}) {
  const hasIconOverrides = options.icons !== undefined;

  return {
    ui: {
      colors: {
        primary: theme.primary,
        neutral: theme.neutral ?? DEFAULT_UI_THEME.neutral,
      },
      card: {
        slots: {
          root: options.cardRoot ?? foundationUiPreset.card.slots.root,
        },
      },
      toast: {
        slots: {
          title: options.toastTitle ?? foundationUiPreset.toast.slots.title,
          description:
            options.toastDescription ??
            foundationUiPreset.toast.slots.description,
        },
      },
      icons: hasIconOverrides
        ? { ...FOUNDATION_TABLER_ICONS, ...options.icons }
        : FOUNDATION_TABLER_ICONS,
    },
  };
}
