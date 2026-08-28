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
  readonly navigationRoot?: string;
  readonly navigationList?: string;
  readonly navigationLink?: string;
  readonly navigationIcon?: string;
}
export interface AdminIconOption {
  label: string;
  value: string;
  keywords?: readonly string[];
}

export interface AdminRowActionItem {
  readonly id: string;
  readonly label: string;
  readonly icon: string;
  readonly to?: string;
  readonly target?: string;
  readonly rel?: string;
  readonly disabled?: boolean;
  readonly loading?: boolean;
  readonly hidden?: boolean;
  readonly tone?: "neutral" | "danger";
  readonly onSelect?: () => void;
}
