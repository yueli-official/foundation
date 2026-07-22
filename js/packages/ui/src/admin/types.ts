import type {
  CommandPaletteGroup,
  CommandPaletteItem,
  NavigationMenuItem,
} from "@nuxt/ui";

export type AdminNavigationItem = NavigationMenuItem;
export type AdminSearchItem = CommandPaletteItem;
export type AdminSearchGroup = CommandPaletteGroup<AdminSearchItem>;

export interface AdminShellMessages {
  readonly skipToContent: string;
  readonly search: string;
  readonly searchPlaceholder: string;
}
