import type {
  AdminNavigationItem,
  AdminSearchGroup,
  AdminShellMessages,
} from "../admin/types";

export type AdminRouteMatch = "exact" | "prefix";
export type AdminModulePlacement = "primary" | "secondary";

export interface AdminAccessRequirement {
  readonly all?: readonly string[];
  readonly any?: readonly string[];
}

interface AdminModuleBase {
  readonly id: string;
  readonly label: string;
  readonly icon: string;
  readonly match?: AdminRouteMatch;
  readonly access?: AdminAccessRequirement;
  readonly search?: boolean;
}

export interface AdminModuleChildDefinition extends AdminModuleBase {
  readonly to: string;
}

export interface AdminModuleDefinition extends AdminModuleBase {
  readonly to?: string;
  readonly placement?: AdminModulePlacement;
  readonly children?: readonly AdminModuleChildDefinition[];
}

export interface AdminProductBrand {
  readonly label: string;
  readonly icon: string;
  readonly to: string;
}

export interface AdminProductDefinition {
  readonly id: string;
  readonly brand: AdminProductBrand;
  readonly modules: readonly AdminModuleDefinition[];
  readonly vocabulary?: Readonly<Record<string, string>>;
  readonly messages?: Partial<AdminShellMessages>;
  readonly searchGroupLabel?: string;
}

export interface AdminProductProjection {
  readonly navigation: readonly AdminNavigationItem[];
  readonly secondaryNavigation: readonly AdminNavigationItem[];
  readonly searchGroups: readonly AdminSearchGroup[];
  readonly currentLabel: string;
}

export interface AdminProductProjectionContext {
  readonly path: string;
  readonly can: (capability: string) => boolean;
}
