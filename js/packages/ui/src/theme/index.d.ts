export interface UiTheme {
  /** Nuxt UI color name registered by the consumer. */
  readonly primary: string;
  /** Nuxt UI neutral palette. Defaults to stone. */
  readonly neutral?: string;
}

export interface UiIconMap {
  readonly [key: string]: string;
  readonly arrowLeft: string;
  readonly arrowRight: string;
  readonly check: string;
  readonly chevronDoubleLeft: string;
  readonly chevronDoubleRight: string;
  readonly chevronUpDown: string;
  readonly chevronDown: string;
  readonly chevronLeft: string;
  readonly chevronRight: string;
  readonly chevronUp: string;
  readonly close: string;
  readonly ellipsis: string;
  readonly external: string;
  readonly loading: string;
  readonly menu: string;
  readonly minus: string;
  readonly panelClose: string;
  readonly panelOpen: string;
  readonly plus: string;
  readonly search: string;
  readonly light: string;
  readonly dark: string;
  readonly system: string;
}

export interface UiPresetOptions {
  readonly icons?: Partial<UiIconMap>;
  readonly cardRoot?: string;
  readonly toastTitle?: string;
  readonly toastDescription?: string;
}

export interface UiPreset {
  readonly [key: string]: unknown;
  readonly ui: {
    readonly colors: { readonly primary: string; readonly neutral: string };
    readonly card: { readonly slots: { readonly root: string } };
    readonly toast: {
      readonly slots: {
        readonly root: string;
        readonly wrapper: string;
        readonly title: string;
        readonly description: string;
        readonly icon: string;
        readonly actions: string;
        readonly progress: string;
        readonly close: string;
      };
    };
    readonly toaster: { readonly slots: { readonly viewport: string } };
    readonly icons: UiIconMap;
  };
}

export declare const DEFAULT_UI_THEME: Readonly<Required<UiTheme>>;
export declare const FOUNDATION_TABLER_ICONS: Readonly<UiIconMap>;
export declare const foundationUiPreset: Readonly<{
  colors: Readonly<{ neutral: string }>;
  card: Readonly<{ slots: Readonly<{ root: string }> }>;
  toast: Readonly<{
    slots: Readonly<{
      root: string;
      wrapper: string;
      title: string;
      description: string;
      icon: string;
      actions: string;
      progress: string;
      close: string;
    }>;
  }>;
  toaster: Readonly<{ slots: Readonly<{ viewport: string }> }>;
}>;

export declare function createUiPreset(
  theme?: UiTheme,
  options?: UiPresetOptions,
): UiPreset;
