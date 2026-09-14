import {
  createAdminNavigationSearchItems,
  normalizeAdminNavigation,
} from "../admin/navigation";
import type {
  AdminNavigationChildItem,
  AdminNavigationItem,
  AdminSearchGroup,
} from "../admin/types";
import type {
  AdminAccessRequirement,
  AdminModuleChildDefinition,
  AdminModuleDefinition,
  AdminProductDefinition,
  AdminProductProjection,
  AdminProductProjectionContext,
} from "./types";

const stableID = /^[a-z0-9]+(?:-[a-z0-9]+)*$/u;

function nonEmpty(value: string, field: string) {
  if (!value.trim()) throw new Error(`admin platform: ${field} is required`);
}

function registerModuleIdentity(
  id: string,
  ids: Set<string>,
  scopedID: string,
) {
  nonEmpty(id, "module id");
  if (!stableID.test(id))
    throw new Error(`admin platform: invalid module id ${id}`);
  if (ids.has(scopedID))
    throw new Error(`admin platform: duplicate module id ${scopedID}`);
  ids.add(scopedID);
}

function registerRoute(to: string, routes: Set<string>, scopedID: string) {
  if (!to.startsWith("/"))
    throw new Error(
      `admin platform: module ${scopedID} route must be absolute`,
    );
  if (routes.has(to))
    throw new Error(`admin platform: duplicate module route ${to}`);
  routes.add(to);
}

function validateModule(
  module: AdminModuleDefinition,
  ids: Set<string>,
  routes: Set<string>,
) {
  registerModuleIdentity(module.id, ids, module.id);
  nonEmpty(module.label, `module ${module.id} label`);
  nonEmpty(module.icon, `module ${module.id} icon`);
  if (module.to) registerRoute(module.to, routes, module.id);
  if (!module.to && !module.children?.length) {
    throw new Error(
      `admin platform: module ${module.id} needs a route or children`,
    );
  }
  for (const child of module.children || []) {
    const scopedID = `${module.id}/${child.id}`;
    registerModuleIdentity(child.id, ids, scopedID);
    nonEmpty(child.label, `module ${scopedID} label`);
    nonEmpty(child.icon, `module ${scopedID} icon`);
    registerRoute(child.to, routes, scopedID);
  }
}

export function defineAdminProduct<T extends AdminProductDefinition>(
  definition: T,
): T {
  nonEmpty(definition.id, "product id");
  if (!stableID.test(definition.id))
    throw new Error(`admin platform: invalid product id ${definition.id}`);
  nonEmpty(definition.brand.label, "brand label");
  nonEmpty(definition.brand.icon, "brand icon");
  nonEmpty(definition.brand.to, "brand route");
  const ids = new Set<string>();
  const routes = new Set<string>();
  for (const module of definition.modules) validateModule(module, ids, routes);
  return definition;
}

export function adminModule<T extends AdminModuleDefinition>(definition: T): T {
  return definition;
}

export function adminTerm(
  product: AdminProductDefinition,
  key: string,
  fallback: string,
): string {
  const value = product.vocabulary?.[key]?.trim();
  return value || fallback;
}

function hasAccess(
  requirement: AdminAccessRequirement | undefined,
  can: (capability: string) => boolean,
): boolean {
  if (!requirement) return true;
  if (requirement.all?.some((capability) => !can(capability))) return false;
  if (
    requirement.any?.length &&
    !requirement.any.some((capability) => can(capability))
  )
    return false;
  return true;
}

function routeActive(
  path: string,
  to: string | undefined,
  match: "exact" | "prefix" | undefined,
) {
  if (!to) return false;
  if (match === "exact") return path === to;
  return path === to || path.startsWith(`${to}/`);
}

function projectChild(
  child: AdminModuleChildDefinition,
  context: AdminProductProjectionContext,
): AdminNavigationChildItem | null {
  if (!hasAccess(child.access, context.can)) return null;
  return {
    label: child.label,
    icon: child.icon,
    to: child.to,
    active: routeActive(context.path, child.to, child.match),
  };
}

function projectModule(
  module: AdminModuleDefinition,
  context: AdminProductProjectionContext,
  searchOnly = false,
): AdminNavigationItem | null {
  if (!hasAccess(module.access, context.can)) return null;
  const children = (module.children || [])
    .filter((child) => !searchOnly || child.search !== false)
    .map((child) => projectChild(child, context))
    .filter((item): item is AdminNavigationChildItem => item !== null);
  if (!module.to && !children.length) return null;
  const active =
    routeActive(context.path, module.to, module.match) ||
    children.some((child) => child.active === true);
  return {
    label: module.label,
    icon: module.icon,
    to: module.to,
    active,
    ...(children.length ? { children } : {}),
  };
}

function currentLabel(items: readonly AdminNavigationItem[]): string {
  for (const item of items) {
    const activeChild = item.children?.find((child) => child.active === true);
    if (activeChild?.label) return String(activeChild.label);
    if (item.active && item.label) return String(item.label);
  }
  return "";
}

export function projectAdminProduct(
  product: AdminProductDefinition,
  context: AdminProductProjectionContext,
): AdminProductProjection {
  const primary: AdminNavigationItem[] = [];
  const secondary: AdminNavigationItem[] = [];
  const searchable: AdminNavigationItem[] = [];

  for (const module of product.modules) {
    const item = projectModule(module, context);
    if (!item) continue;
    if ((module.placement || "primary") === "secondary") secondary.push(item);
    else primary.push(item);
    if (module.search !== false) {
      const searchItem = projectModule(module, context, true);
      if (searchItem) searchable.push(searchItem);
    }
  }

  const navigation = normalizeAdminNavigation(primary);
  const secondaryNavigation = normalizeAdminNavigation(secondary);
  const searchItems = createAdminNavigationSearchItems(
    normalizeAdminNavigation(searchable),
    {
      idPrefix: `${product.id}-admin`,
    },
  );
  const searchGroups: AdminSearchGroup[] = searchItems.length
    ? [
        {
          id: `${product.id}-pages`,
          label: product.searchGroupLabel || "页面",
          items: searchItems,
        },
      ]
    : [];

  return {
    navigation,
    secondaryNavigation,
    searchGroups,
    currentLabel: currentLabel([...navigation, ...secondaryNavigation]),
  };
}
