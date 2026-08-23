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
  readonly currentLocation?: string;
}

export interface AdminShellUi {
  readonly sidebar?: string;
  readonly sidebarHeader?: string;
  readonly sidebarBody?: string;
  readonly sidebarFooter?: string;
  readonly sidebarContent?: string;
  readonly search?: string;
  readonly searchButton?: string;
  readonly navigationRoot?: string;
  readonly navigationList?: string;
  readonly navigationLink?: string;
  readonly navigationIcon?: string;
}
