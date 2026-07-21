import type { MessageReference } from "../messages";

export type JsonPrimitive = string | number | boolean | null;
export type JsonArray = readonly JsonValue[];
export type JsonObject = Readonly<{ [key: string]: JsonValue }>;
export type JsonValue = JsonPrimitive | JsonArray | JsonObject;

export type CollectionKey = string | number;
export type CollectionLoadState =
  "idle" | "loading" | "refreshing" | "ready" | "error";
export type CollectionChange = "query" | "load" | "selection";
export type CollectionIssue = MessageReference;

export interface CollectionPage<TItem> {
  readonly items: readonly TItem[];
  readonly total: number;
}

export interface CollectionQueryPolicy<TQuery> {
  snapshot(query: TQuery): Readonly<TQuery>;
  equals(left: Readonly<TQuery>, right: Readonly<TQuery>): boolean;
}

export interface CollectionLoadToken<TQuery> {
  readonly sequence: number;
  readonly query: Readonly<TQuery>;
}

export type CollectionSelectionSnapshot<TKey extends CollectionKey, TQuery> =
  | {
      readonly mode: "keys";
      readonly keys: readonly TKey[];
      readonly count: number;
    }
  | {
      readonly mode: "query";
      readonly query: Readonly<TQuery>;
      readonly excludedKeys: readonly TKey[];
      readonly count: number;
    };

export type CollectionSelectionRequest<TKey extends CollectionKey, TQuery> =
  | { readonly mode: "keys"; readonly keys: readonly TKey[] }
  | {
      readonly mode: "query";
      readonly query: Readonly<TQuery>;
      readonly excludedKeys: readonly TKey[];
    };

export interface CollectionSnapshot<TItem, TKey extends CollectionKey, TQuery> {
  readonly query: Readonly<TQuery>;
  readonly items: readonly TItem[];
  readonly total: number;
  readonly loadState: CollectionLoadState;
  readonly issue?: CollectionIssue;
  readonly selection: CollectionSelectionSnapshot<TKey, TQuery>;
  readonly selectedPageCount: number;
  readonly isPageSelected: boolean;
  readonly isPageIndeterminate: boolean;
}

export interface CollectionWorkflowOptions<
  TItem,
  TKey extends CollectionKey,
  TQuery,
> {
  readonly initialQuery: TQuery;
  readonly queryPolicy: CollectionQueryPolicy<TQuery>;
  readonly keyOf: (item: TItem) => TKey;
  readonly initialPage?: CollectionPage<TItem>;
}

export interface CollectionWorkflow<TItem, TKey extends CollectionKey, TQuery> {
  getSnapshot(): CollectionSnapshot<TItem, TKey, TQuery>;
  subscribe(
    listener: (
      snapshot: CollectionSnapshot<TItem, TKey, TQuery>,
      change: CollectionChange,
    ) => void,
  ): () => void;
  setQuery(query: TQuery): boolean;
  beginLoad(): CollectionLoadToken<TQuery>;
  resolveLoad(
    token: CollectionLoadToken<TQuery>,
    page: CollectionPage<TItem>,
  ): boolean;
  rejectLoad(
    token: CollectionLoadToken<TQuery>,
    issue: CollectionIssue,
  ): boolean;
  isSelected(key: TKey): boolean;
  toggleKey(key: TKey): void;
  togglePage(selected?: boolean): void;
  selectAllResults(): void;
  clearSelection(): void;
  getSelectionRequest(): CollectionSelectionRequest<TKey, TQuery>;
  dispose(): void;
}

export interface CollectionQuerySync<TQuery> {
  read(): TQuery;
  replace(query: Readonly<TQuery>): void | Promise<void>;
  subscribe(listener: (query: TQuery) => void): () => void;
}

function cloneJson(
  value: JsonValue,
  path: string,
  seen: Set<object>,
): JsonValue {
  if (value === null || typeof value === "string" || typeof value === "boolean")
    return value;
  if (typeof value === "number") {
    if (!Number.isFinite(value))
      throw new TypeError(
        `Collection query contains a non-finite number at ${path}.`,
      );
    return value;
  }
  if (typeof value !== "object")
    throw new TypeError(
      `Collection query contains a non-JSON value at ${path}.`,
    );
  if (seen.has(value))
    throw new TypeError(`Collection query contains a cycle at ${path}.`);

  seen.add(value);
  let result: JsonValue;
  if (Array.isArray(value)) {
    result = Object.freeze(
      value.map((entry, index) => cloneJson(entry, `${path}/${index}`, seen)),
    );
  } else {
    const prototype = Object.getPrototypeOf(value);
    if (prototype !== Object.prototype && prototype !== null) {
      throw new TypeError(
        `Collection query contains a non-plain object at ${path}.`,
      );
    }
    const objectValue = value as JsonObject;
    result = Object.freeze(
      Object.fromEntries(
        Object.keys(objectValue)
          .sort()
          .map((key) => [
            key,
            cloneJson(objectValue[key]!, `${path}/${key}`, seen),
          ]),
      ),
    );
  }
  seen.delete(value);
  return result;
}

export function createJsonCollectionQueryPolicy<
  TQuery extends object,
>(): CollectionQueryPolicy<TQuery> {
  return {
    snapshot(query) {
      return cloneJson(
        query as unknown as JsonValue,
        "",
        new Set(),
      ) as unknown as Readonly<TQuery>;
    },
    equals(left, right) {
      const normalizedLeft = cloneJson(
        left as unknown as JsonValue,
        "",
        new Set(),
      );
      const normalizedRight = cloneJson(
        right as unknown as JsonValue,
        "",
        new Set(),
      );
      return JSON.stringify(normalizedLeft) === JSON.stringify(normalizedRight);
    },
  };
}

function normalizePage<TItem, TKey extends CollectionKey>(
  page: CollectionPage<TItem>,
  keyOf: (item: TItem) => TKey,
): {
  readonly items: readonly TItem[];
  readonly total: number;
  readonly keys: readonly TKey[];
} {
  if (!Number.isSafeInteger(page.total) || page.total < 0) {
    throw new RangeError(
      "Collection page total must be a non-negative safe integer.",
    );
  }

  const seen = new Set<TKey>();
  const items: TItem[] = [];
  const keys: TKey[] = [];
  for (const item of page.items) {
    const key = keyOf(item);
    if (typeof key === "number" && !Number.isFinite(key)) {
      throw new TypeError(
        "Collection item keys must be finite strings or numbers.",
      );
    }
    if (seen.has(key)) continue;
    seen.add(key);
    items.push(item);
    keys.push(key);
  }

  if (page.total < items.length) {
    throw new RangeError(
      "Collection page total cannot be smaller than the number of unique visible items.",
    );
  }

  return {
    items: Object.freeze(items),
    total: page.total,
    keys: Object.freeze(keys),
  };
}

export function createCollectionWorkflow<
  TItem,
  TKey extends CollectionKey,
  TQuery,
>(
  options: CollectionWorkflowOptions<TItem, TKey, TQuery>,
): CollectionWorkflow<TItem, TKey, TQuery> {
  const listeners = new Set<
    (
      snapshot: CollectionSnapshot<TItem, TKey, TQuery>,
      change: CollectionChange,
    ) => void
  >();
  let disposed = false;
  let query = options.queryPolicy.snapshot(options.initialQuery);
  let items: readonly TItem[] = Object.freeze([]);
  let visibleKeys: readonly TKey[] = Object.freeze([]);
  let total = 0;
  let loadState: CollectionLoadState = "idle";
  let issue: CollectionIssue | undefined;
  let sequence = 0;
  let activeSequence = 0;
  let selectedKeys = new Set<TKey>();
  let querySelection:
    { query: Readonly<TQuery>; excludedKeys: Set<TKey> } | undefined;

  if (options.initialPage) {
    const normalized = normalizePage(options.initialPage, options.keyOf);
    items = normalized.items;
    visibleKeys = normalized.keys;
    total = normalized.total;
    loadState = "ready";
  }

  function assertActive() {
    if (disposed) throw new Error("Collection workflow has been disposed.");
  }

  function selectionSnapshot(): CollectionSelectionSnapshot<TKey, TQuery> {
    if (querySelection) {
      const excludedKeys = Object.freeze([...querySelection.excludedKeys]);
      return Object.freeze({
        mode: "query" as const,
        query: querySelection.query,
        excludedKeys,
        count: Math.max(0, total - excludedKeys.length),
      });
    }
    const keys = Object.freeze([...selectedKeys]);
    return Object.freeze({ mode: "keys" as const, keys, count: keys.length });
  }

  function selected(key: TKey): boolean {
    return querySelection
      ? !querySelection.excludedKeys.has(key)
      : selectedKeys.has(key);
  }

  function snapshot(): CollectionSnapshot<TItem, TKey, TQuery> {
    const selectedPageCount = visibleKeys.filter(selected).length;
    return Object.freeze({
      query,
      items,
      total,
      loadState,
      ...(issue ? { issue } : {}),
      selection: selectionSnapshot(),
      selectedPageCount,
      isPageSelected:
        visibleKeys.length > 0 && selectedPageCount === visibleKeys.length,
      isPageIndeterminate:
        selectedPageCount > 0 && selectedPageCount < visibleKeys.length,
    });
  }

  function emit(change: CollectionChange) {
    const next = snapshot();
    for (const listener of [...listeners]) listener(next, change);
  }

  function clearSelectionState(): boolean {
    if (!querySelection && selectedKeys.size === 0) return false;
    querySelection = undefined;
    selectedKeys = new Set();
    return true;
  }

  return {
    getSnapshot() {
      return snapshot();
    },
    subscribe(listener) {
      assertActive();
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    setQuery(nextQuery) {
      assertActive();
      const next = options.queryPolicy.snapshot(nextQuery);
      if (options.queryPolicy.equals(query, next)) return false;
      query = next;
      activeSequence = ++sequence;
      clearSelectionState();
      issue = undefined;
      emit("query");
      return true;
    },
    beginLoad() {
      assertActive();
      const token = Object.freeze({ sequence: ++sequence, query });
      activeSequence = token.sequence;
      issue = undefined;
      loadState = items.length > 0 ? "refreshing" : "loading";
      emit("load");
      return token;
    },
    resolveLoad(token, page) {
      assertActive();
      if (
        token.sequence !== activeSequence ||
        !options.queryPolicy.equals(token.query, query)
      )
        return false;
      const normalized = normalizePage(page, options.keyOf);
      items = normalized.items;
      visibleKeys = normalized.keys;
      total = normalized.total;
      issue = undefined;
      loadState = "ready";
      emit("load");
      return true;
    },
    rejectLoad(token, nextIssue) {
      assertActive();
      if (
        token.sequence !== activeSequence ||
        !options.queryPolicy.equals(token.query, query)
      )
        return false;
      issue = Object.freeze({
        ...nextIssue,
        ...(nextIssue.params
          ? { params: Object.freeze({ ...nextIssue.params }) }
          : {}),
      });
      loadState = "error";
      emit("load");
      return true;
    },
    isSelected(key) {
      return selected(key);
    },
    toggleKey(key) {
      assertActive();
      if (querySelection) {
        if (querySelection.excludedKeys.has(key))
          querySelection.excludedKeys.delete(key);
        else querySelection.excludedKeys.add(key);
      } else if (selectedKeys.has(key)) selectedKeys.delete(key);
      else selectedKeys.add(key);
      emit("selection");
    },
    togglePage(forceSelected) {
      assertActive();
      const pageIsSelected =
        visibleKeys.length > 0 && visibleKeys.every(selected);
      const shouldSelect = forceSelected ?? !pageIsSelected;
      if (querySelection) {
        for (const key of visibleKeys) {
          if (shouldSelect) querySelection.excludedKeys.delete(key);
          else querySelection.excludedKeys.add(key);
        }
      } else {
        for (const key of visibleKeys) {
          if (shouldSelect) selectedKeys.add(key);
          else selectedKeys.delete(key);
        }
      }
      emit("selection");
    },
    selectAllResults() {
      assertActive();
      selectedKeys = new Set();
      querySelection = {
        query: options.queryPolicy.snapshot(query as TQuery),
        excludedKeys: new Set(),
      };
      emit("selection");
    },
    clearSelection() {
      assertActive();
      if (clearSelectionState()) emit("selection");
    },
    getSelectionRequest() {
      if (querySelection) {
        return Object.freeze({
          mode: "query" as const,
          query: querySelection.query,
          excludedKeys: Object.freeze([...querySelection.excludedKeys]),
        });
      }
      return Object.freeze({
        mode: "keys" as const,
        keys: Object.freeze([...selectedKeys]),
      });
    },
    dispose() {
      disposed = true;
      activeSequence = ++sequence;
      listeners.clear();
    },
  };
}

export function createMemoryCollectionQuerySync<TQuery>(
  initialQuery: TQuery,
  policy: CollectionQueryPolicy<TQuery>,
): CollectionQuerySync<TQuery> & { history(): readonly Readonly<TQuery>[] } {
  const listeners = new Set<(query: TQuery) => void>();
  let current = policy.snapshot(initialQuery);
  const entries: Readonly<TQuery>[] = [current];

  return {
    read: () => current as TQuery,
    replace(next) {
      if (policy.equals(current, next)) return;
      current = policy.snapshot(next as TQuery);
      entries.push(current);
      for (const listener of [...listeners]) listener(current as TQuery);
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    history: () => Object.freeze([...entries]),
  };
}

export function bindCollectionQuerySync<
  TItem,
  TKey extends CollectionKey,
  TQuery,
>(
  workflow: CollectionWorkflow<TItem, TKey, TQuery>,
  sync: CollectionQuerySync<TQuery>,
): () => void {
  let applyingExternal = false;
  let pendingLocalWrites = 0;
  let deferredExternal: TQuery | undefined;
  let disposed = false;

  function applyExternal(next: TQuery) {
    if (disposed) return;
    applyingExternal = true;
    try {
      workflow.setQuery(next);
    } finally {
      applyingExternal = false;
    }
  }

  function settleLocalWrite() {
    pendingLocalWrites = Math.max(0, pendingLocalWrites - 1);
    if (disposed || pendingLocalWrites > 0) return;
    const next = deferredExternal ?? sync.read();
    deferredExternal = undefined;
    applyExternal(next);
  }

  workflow.setQuery(sync.read());

  const unsubscribeSync = sync.subscribe((next) => {
    if (pendingLocalWrites > 0) {
      deferredExternal = next;
      return;
    }
    applyExternal(next);
  });
  const unsubscribeWorkflow = workflow.subscribe((next, change) => {
    if (change !== "query" || applyingExternal) return;
    pendingLocalWrites += 1;
    try {
      const replacement = sync.replace(next.query);
      if (replacement && typeof replacement.then === "function") {
        void replacement.catch(() => undefined).finally(settleLocalWrite);
      } else {
        settleLocalWrite();
      }
    } catch {
      settleLocalWrite();
    }
  });

  return () => {
    disposed = true;
    unsubscribeWorkflow();
    unsubscribeSync();
  };
}
