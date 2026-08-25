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
  chevronUpDown: "i-tabler-selector",
  chevronDown: "i-tabler-chevron-down",
  chevronLeft: "i-tabler-chevron-left",
  chevronRight: "i-tabler-chevron-right",
  chevronUp: "i-tabler-chevron-up",
  close: "i-tabler-x",
  ellipsis: "i-tabler-dots",
  external: "i-tabler-external-link",
  loading: "i-tabler-loader-2",
  menu: "i-tabler-menu-2",
  minus: "i-tabler-minus",
  panelClose: "i-tabler-layout-sidebar-left-collapse",
  panelOpen: "i-tabler-layout-sidebar-left-expand",
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
      root: "yueli-toast w-full gap-3 rounded-xl border border-default bg-default p-3 shadow-[0_14px_36px_rgb(15_23_42/0.14)] ring-0",
      wrapper: "min-w-0 flex-1",
      title: "line-clamp-2 text-sm font-semibold leading-5 text-highlighted",
      description: "mt-0.5 line-clamp-2 text-xs leading-5 text-muted",
      icon: "mt-0.5 size-5 shrink-0",
      actions: "shrink-0 items-start gap-1.5",
      progress: "hidden",
      close: "-mr-1 -mt-1 p-1 text-dimmed hover:text-default",
    }),
  }),
  toaster: Object.freeze({
    slots: Object.freeze({
      viewport: "sm:w-[22rem]",
    }),
  }),
  input: Object.freeze({ slots: Object.freeze({ base: "yueli-field-border" }) }),
  inputNumber: Object.freeze({ slots: Object.freeze({ base: "yueli-field-border" }) }),
  textarea: Object.freeze({ slots: Object.freeze({ base: "yueli-field-border" }) }),
  select: Object.freeze({ slots: Object.freeze({ base: "yueli-field-border" }) }),
  selectMenu: Object.freeze({
    slots: Object.freeze({
      base: "yueli-field-border",
      input: "yueli-select-menu-search",
    }),
  }),
  inputMenu: Object.freeze({ slots: Object.freeze({ base: "yueli-field-border" }) }),
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
          ...foundationUiPreset.toast.slots,
          title: options.toastTitle ?? foundationUiPreset.toast.slots.title,
          description:
            options.toastDescription ??
            foundationUiPreset.toast.slots.description,
        },
      },
      toaster: foundationUiPreset.toaster,
      input: foundationUiPreset.input,
      inputNumber: foundationUiPreset.inputNumber,
      textarea: foundationUiPreset.textarea,
      select: foundationUiPreset.select,
      selectMenu: foundationUiPreset.selectMenu,
      inputMenu: foundationUiPreset.inputMenu,
      icons: hasIconOverrides
        ? { ...FOUNDATION_TABLER_ICONS, ...options.icons }
        : FOUNDATION_TABLER_ICONS,
    },
  };
}
