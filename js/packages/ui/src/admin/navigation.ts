import type {
  AdminNavigationChildItem,
  AdminNavigationItem,
  AdminSearchItem,
} from "./types";

export interface AdminNavigationSearchOptions {
  readonly idPrefix?: string;
  readonly childLabelSeparator?: string;
  readonly includeParentLinks?: boolean;
}

function hasActiveChild(children: readonly AdminNavigationChildItem[]) {
  return children.some((child) => child.active === true);
}

/**
 * Clone a bounded two-level navigation tree and derive parent state from an
 * explicitly active child. Route and permission decisions remain caller-owned.
 */
export function normalizeAdminNavigation(
  items: readonly AdminNavigationItem[],
): AdminNavigationItem[] {
  return items.map((item) => {
    const children = item.children?.map((child) => ({ ...child }));
    if (!children?.length) return { ...item };

    const childActive = hasActiveChild(children);
    return {
      ...item,
      to: undefined,
      target: undefined,
      external: undefined,
      onSelect: undefined,
      type: "trigger",
      slot: item.slot || "admin-navigation-group",
      children,
      ...(childActive ? { active: true } : {}),
      ...(childActive &&
      item.defaultOpen === undefined &&
      item.open === undefined
        ? { defaultOpen: true }
        : {}),
    };
  });
}

function searchItem(
  item: AdminNavigationItem | AdminNavigationChildItem,
  id: string,
  label: string,
): AdminSearchItem {
  return {
    id,
    label,
    icon: item.icon,
    to: item.to,
    target: item.target,
    disabled: item.disabled,
  };
}

/**
 * Flatten navigable leaves for the admin command palette. Trigger-only parents
 * are excluded, while child labels keep their parent context.
 */
export function createAdminNavigationSearchItems(
  items: readonly AdminNavigationItem[],
  options: AdminNavigationSearchOptions = {},
): AdminSearchItem[] {
  const idPrefix = options.idPrefix || "admin-page";
  const separator = options.childLabelSeparator || " · ";
  const projected: AdminSearchItem[] = [];

  items.forEach((item, itemIndex) => {
    const itemLabel = item.label?.trim();
    const children = item.children || [];
    const itemId = `${idPrefix}-${itemIndex}`;

    if (!children.length) {
      if (itemLabel && item.to) {
        projected.push(searchItem(item, itemId, itemLabel));
      }
      return;
    }

    if (
      options.includeParentLinks &&
      item.type !== "trigger" &&
      itemLabel &&
      item.to
    ) {
      projected.push(searchItem(item, itemId, itemLabel));
    }

    children.forEach((child, childIndex) => {
      const childLabel = child.label?.trim();
      if (!childLabel || !child.to) return;
      projected.push(
        searchItem(
          child,
          `${itemId}-${childIndex}`,
          itemLabel ? `${itemLabel}${separator}${childLabel}` : childLabel,
        ),
      );
    });
  });

  return projected;
}
