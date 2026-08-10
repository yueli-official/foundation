import type {
  CommandPaletteGroup,
  CommandPaletteItem,
  NavigationMenuItem,
} from "@nuxt/ui";

export type AdminNavigationChildItem = Omit<NavigationMenuItem, "children"> & {
  readonly children?: never;
};

export type AdminNavigationItem = Omit<NavigationMenuItem, "children"> & {
  readonly children?: AdminNavigationChildItem[];
};
export type AdminSearchItem = CommandPaletteItem;
export type AdminSearchGroup = CommandPaletteGroup<AdminSearchItem>;

export interface AdminShellMessages {
  readonly skipToContent: string;
  readonly search: string;
  readonly searchPlaceholder: string;
}
