export type AdminClassificationStatus =
  "draft" | "active" | "inactive" | "replaced";

/**
 * Canonical admin projection of a category. Products may extend this view with
 * fields such as icons, media references, revisions, or content counters.
 */
export interface AdminClassificationCategory {
  readonly id: string;
  readonly parentId?: string;
  readonly slug: string;
  readonly name: string;
  readonly status: AdminClassificationStatus;
  readonly editorialPosition?: number | null;
  readonly description?: string;
  readonly searchText?: string;
}

/** Canonical admin projection of a Classification Facet. */
export interface AdminClassificationFacet {
  readonly id: string;
  readonly slug: string;
  readonly name: string;
  readonly status: AdminClassificationStatus;
  readonly editorialPosition?: number | null;
  readonly description?: string;
  readonly searchText?: string;
}

/** Canonical admin projection of the assignment policy attached to one Facet. */
export interface AdminClassificationFacetPolicy {
  readonly facetId: string;
  readonly minValues: number;
  readonly maxValues: number;
  readonly leafOnly: boolean;
  readonly maxDepth: number;
}

/** Canonical admin projection of one recursively nested Facet Value. */
export interface AdminClassificationFacetValue {
  readonly id: string;
  readonly facetId: string;
  readonly parentId?: string;
  readonly slug: string;
  readonly name: string;
  readonly status: AdminClassificationStatus;
  readonly editorialPosition?: number | null;
  readonly description?: string;
  readonly searchText?: string;
}

export interface AdminClassificationCategoryRow<
  T extends AdminClassificationCategory = AdminClassificationCategory,
> {
  readonly item: T;
  readonly id: string;
  readonly parentId: string;
  readonly depth: number;
  readonly ancestorIds: readonly string[];
  readonly rootId: string;
  readonly pathLabel: string;
  readonly hasChildren: boolean;
}

export interface AdminClassificationFacetValueRow<
  T extends AdminClassificationFacetValue = AdminClassificationFacetValue,
> {
  readonly item: T;
  readonly id: string;
  readonly facetId: string;
  readonly parentId: string;
  readonly depth: number;
  readonly ancestorIds: readonly string[];
  readonly rootId: string;
  readonly pathLabel: string;
  readonly hasChildren: boolean;
}

export interface AdminCategoryCollectionQuery {
  readonly search?: string;
  readonly status?: AdminClassificationStatus | "all";
  readonly page?: number;
  readonly pageSize?: number;
  readonly collapsedIds?: ReadonlySet<string> | readonly string[];
}

export interface AdminCategoryCollectionProjection<
  T extends AdminClassificationCategory = AdminClassificationCategory,
> {
  readonly rows: readonly AdminClassificationCategoryRow<T>[];
  readonly visibleRows: readonly AdminClassificationCategoryRow<T>[];
  readonly branchIds: ReadonlySet<string>;
  readonly filtering: boolean;
  readonly matchCount: number;
  readonly totalCount: number;
  readonly rootCount: number;
  readonly page: number;
  readonly pageSize: number;
  readonly pageCount: number;
}

export interface AdminCategoryParentOption {
  readonly label: string;
  readonly value: string;
}

export interface AdminCategoryParentOptionsConfig {
  readonly currentId?: string;
  readonly rootLabel?: string;
  readonly rootValue?: string;
}

export interface AdminFacetValueParentOptionsConfig {
  readonly facetId: string;
  readonly currentId?: string;
  readonly rootLabel?: string;
  readonly rootValue?: string;
}

export interface AdminClassificationTag {
  readonly id: string;
  readonly name: string;
  readonly status: AdminClassificationStatus;
  readonly description?: string;
  readonly usageCount?: number;
  readonly searchText?: string;
}

export type AdminTagSort = "usage" | "name";

export interface AdminTagCollectionQuery {
  readonly search?: string;
  readonly status?: AdminClassificationStatus | "all";
  readonly sort?: AdminTagSort;
  readonly page?: number;
  readonly pageSize?: number;
}

export interface AdminTagCollectionProjection<
  T extends AdminClassificationTag = AdminClassificationTag,
> {
  readonly rows: readonly T[];
  readonly visibleRows: readonly T[];
  readonly totalCount: number;
  readonly page: number;
  readonly pageSize: number;
  readonly pageCount: number;
}

function normalizedPage(value: number | undefined): number {
  return Number.isInteger(value) && Number(value) > 0 ? Number(value) : 1;
}

function normalizedPageSize(value: number | undefined): number {
  return Number.isInteger(value) && Number(value) > 0 ? Number(value) : 20;
}

function position(value: number | null | undefined): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

interface AdminHierarchyItem {
  readonly id: string;
  readonly parentId?: string;
  readonly name: string;
  readonly editorialPosition?: number | null;
}

interface AdminHierarchyRow<T extends AdminHierarchyItem> {
  readonly item: T;
  readonly id: string;
  readonly parentId: string;
  readonly depth: number;
  readonly ancestorIds: readonly string[];
  readonly rootId: string;
  readonly pathLabel: string;
  readonly hasChildren: boolean;
}

function compareHierarchyItems(
  a: AdminHierarchyItem,
  b: AdminHierarchyItem,
): number {
  return (
    position(a.editorialPosition) - position(b.editorialPosition) ||
    a.name.localeCompare(b.name) ||
    a.id.localeCompare(b.id)
  );
}

function categorySearchText(item: AdminClassificationCategory): string {
  return `${item.name} ${item.slug} ${item.description || ""} ${item.searchText || ""}`.toLocaleLowerCase();
}

function tagSearchText(item: AdminClassificationTag): string {
  return `${item.name} ${item.id} ${item.description || ""} ${item.searchText || ""}`.toLocaleLowerCase();
}

function asCollapsedSet(
  value: ReadonlySet<string> | readonly string[] | undefined,
): ReadonlySet<string> {
  if (!value) return new Set<string>();
  return value instanceof Set ? value : new Set(value);
}

function buildAdminHierarchyRows<T extends AdminHierarchyItem>(
  items: readonly T[],
  kind: "category" | "facet value",
  parentAllowed: (item: T, parent: T) => boolean = () => true,
): readonly AdminHierarchyRow<T>[] {
  const byId = new Map<string, T>();
  for (const item of items) {
    if (!item.id.trim())
      throw new Error(`admin classification: ${kind} id is required`);
    if (byId.has(item.id)) {
      throw new Error(`admin classification: duplicate ${kind} id ${item.id}`);
    }
    byId.set(item.id, item);
  }

  const children = new Map<string, T[]>();
  for (const item of byId.values()) {
    const requestedParent = item.parentId?.trim() || "";
    const parent = requestedParent ? byId.get(requestedParent) : undefined;
    const parentId =
      parent && requestedParent !== item.id && parentAllowed(item, parent)
        ? requestedParent
        : "";
    const group = children.get(parentId) || [];
    group.push(item);
    children.set(parentId, group);
  }
  for (const group of children.values()) group.sort(compareHierarchyItems);

  const branchIds = new Set(
    [...children.entries()]
      .filter(([parentId, group]) => parentId && group.length)
      .map(([parentId]) => parentId),
  );
  const rows: AdminHierarchyRow<T>[] = [];
  const visited = new Set<string>();

  const visit = (roots: readonly T[]) => {
    const pending = [...roots].reverse().map((item) => ({
      item,
      names: [] as string[],
      ancestors: [] as string[],
    }));
    while (pending.length) {
      const current = pending.pop();
      if (!current || visited.has(current.item.id)) continue;
      visited.add(current.item.id);
      const names = [...current.names, current.item.name];
      const ancestorIds = current.ancestors;
      rows.push({
        item: current.item,
        id: current.item.id,
        parentId: current.item.parentId?.trim() || "",
        depth: ancestorIds.length,
        ancestorIds,
        rootId: ancestorIds[0] || current.item.id,
        pathLabel: names.join(" / "),
        hasChildren: branchIds.has(current.item.id),
      });
      const childRows = children.get(current.item.id) || [];
      for (const child of [...childRows].reverse()) {
        pending.push({
          item: child,
          names,
          ancestors: [...ancestorIds, current.item.id],
        });
      }
    }
  };

  visit(children.get("") || []);
  // A malformed cycle can have no root. Keep it visible without looping.
  for (const item of [...byId.values()].sort(compareHierarchyItems)) {
    if (!visited.has(item.id)) visit([item]);
  }
  return rows;
}

/**
 * Builds a deterministic hierarchy while keeping malformed orphan/cycle rows
 * discoverable. Duplicate ids are rejected because they make category identity
 * ambiguous and violate the shared Classification contract.
 */
export function buildAdminCategoryTree<T extends AdminClassificationCategory>(
  items: readonly T[],
): readonly AdminClassificationCategoryRow<T>[] {
  return buildAdminHierarchyRows(items, "category");
}

/**
 * Builds the recursive value tree for one or many Facets. When facetId is
 * supplied, only that Facet is projected. A cross-Facet parent is treated like
 * an orphan so malformed data remains visible without leaking another Facet
 * into the hierarchy.
 */
export function buildAdminFacetValueTree<
  T extends AdminClassificationFacetValue,
>(
  items: readonly T[],
  facetId?: string,
): readonly AdminClassificationFacetValueRow<T>[] {
  const scope = facetId?.trim() || "";
  const scopedItems = scope
    ? items.filter((item) => item.facetId === scope)
    : items;
  return buildAdminHierarchyRows(
    scopedItems,
    "facet value",
    (item, parent) => item.facetId === parent.facetId,
  ).map((row) => ({ ...row, facetId: row.item.facetId }));
}

export function projectAdminCategoryCollection<
  T extends AdminClassificationCategory,
>(
  items: readonly T[],
  query: AdminCategoryCollectionQuery = {},
): AdminCategoryCollectionProjection<T> {
  const tree = buildAdminCategoryTree(items);
  const search = query.search?.trim().toLocaleLowerCase() || "";
  const status = query.status || "all";
  const filtering = Boolean(search) || status !== "all";
  const matches = new Set(
    tree
      .filter(
        (row) =>
          (!search || categorySearchText(row.item).includes(search)) &&
          (status === "all" || row.item.status === status),
      )
      .map((row) => row.id),
  );
  const included = new Set<string>();
  for (const row of tree) {
    if (!matches.has(row.id)) continue;
    included.add(row.id);
    for (const ancestorId of row.ancestorIds) included.add(ancestorId);
  }
  const rows = tree.filter((row) => included.has(row.id));
  const rootRows = rows.filter((row) => row.depth === 0);
  const pageSize = normalizedPageSize(query.pageSize);
  const pageCount = Math.max(1, Math.ceil(rootRows.length / pageSize));
  const page = Math.min(normalizedPage(query.page), pageCount);
  const selectedRoots = new Set(
    rootRows.slice((page - 1) * pageSize, page * pageSize).map((row) => row.id),
  );
  const collapsedIds = asCollapsedSet(query.collapsedIds);
  const visibleRows = rows.filter(
    (row) =>
      selectedRoots.has(row.rootId) &&
      (filtering || !row.ancestorIds.some((id) => collapsedIds.has(id))),
  );
  const branchIds = new Set(
    tree.filter((row) => row.hasChildren).map((row) => row.id),
  );

  return {
    rows,
    visibleRows,
    branchIds,
    filtering,
    matchCount: matches.size,
    totalCount: tree.length,
    rootCount: rootRows.length,
    page,
    pageSize,
    pageCount,
  };
}

/**
 * Produces path labels for a parent selector and excludes the current category
 * plus all of its descendants, preventing a product UI from offering a cycle.
 */
export function createAdminCategoryParentOptions<
  T extends AdminClassificationCategory,
>(
  items: readonly T[],
  config: AdminCategoryParentOptionsConfig = {},
): readonly AdminCategoryParentOption[] {
  const rows = buildAdminCategoryTree(items);
  const currentId = config.currentId?.trim() || "";
  const excluded = new Set<string>();
  if (currentId) {
    excluded.add(currentId);
    for (const row of rows) {
      if (row.ancestorIds.includes(currentId)) excluded.add(row.id);
    }
  }
  return [
    { label: config.rootLabel || "Root", value: config.rootValue || "" },
    ...rows
      .filter((row) => !excluded.has(row.id))
      .map((row) => ({ label: row.pathLabel, value: row.id })),
  ];
}

/**
 * Produces path-labelled parent choices inside one Facet and excludes the
 * current value plus all descendants, preventing a recursive move from
 * introducing a cycle.
 */
export function createAdminFacetValueParentOptions<
  T extends AdminClassificationFacetValue,
>(
  items: readonly T[],
  config: AdminFacetValueParentOptionsConfig,
): readonly AdminCategoryParentOption[] {
  const rows = buildAdminFacetValueTree(items, config.facetId);
  const currentId = config.currentId?.trim() || "";
  const excluded = new Set<string>();
  if (currentId) {
    excluded.add(currentId);
    for (const row of rows) {
      if (row.ancestorIds.includes(currentId)) excluded.add(row.id);
    }
  }
  return [
    { label: config.rootLabel || "Root", value: config.rootValue || "" },
    ...rows
      .filter((row) => !excluded.has(row.id))
      .map((row) => ({ label: row.pathLabel, value: row.id })),
  ];
}

export function projectAdminTagCollection<T extends AdminClassificationTag>(
  items: readonly T[],
  query: AdminTagCollectionQuery = {},
): AdminTagCollectionProjection<T> {
  const search = query.search?.trim().toLocaleLowerCase() || "";
  const status = query.status || "all";
  const sort = query.sort || "usage";
  const rows = items.filter(
    (item) =>
      (!search || tagSearchText(item).includes(search)) &&
      (status === "all" || item.status === status),
  );
  rows.sort((a, b) => {
    if (sort === "name") {
      return a.name.localeCompare(b.name) || a.id.localeCompare(b.id);
    }
    return (
      (b.usageCount || 0) - (a.usageCount || 0) ||
      a.name.localeCompare(b.name) ||
      a.id.localeCompare(b.id)
    );
  });
  const pageSize = normalizedPageSize(query.pageSize);
  const pageCount = Math.max(1, Math.ceil(rows.length / pageSize));
  const page = Math.min(normalizedPage(query.page), pageCount);
  return {
    rows,
    visibleRows: rows.slice((page - 1) * pageSize, page * pageSize),
    totalCount: rows.length,
    page,
    pageSize,
    pageCount,
  };
}
