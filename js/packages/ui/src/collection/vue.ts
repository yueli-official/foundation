import { onMounted, onScopeDispose, shallowRef, type ShallowRef } from "vue";
import {
  bindCollectionQuerySync,
  createCollectionWorkflow,
  type CollectionKey,
  type CollectionQuerySync,
  type CollectionSnapshot,
  type CollectionWorkflow,
  type CollectionWorkflowOptions,
} from "./workflow";

export type CollectionDataQueryKey = string | number | boolean | null;

export interface VueCollectionWorkflowOptions<
  TItem,
  TKey extends CollectionKey,
  TQuery,
> extends CollectionWorkflowOptions<TItem, TKey, TQuery> {
  readonly querySync?: CollectionQuerySync<TQuery>;
  readonly dataQueryKey?: (query: Readonly<TQuery>) => CollectionDataQueryKey;
  readonly load: (
    query: Readonly<TQuery>,
    workflow: CollectionWorkflow<TItem, TKey, TQuery>,
  ) => Promise<void>;
}

export interface VueCollectionWorkflow<
  TItem,
  TKey extends CollectionKey,
  TQuery,
> {
  readonly snapshot: Readonly<
    ShallowRef<CollectionSnapshot<TItem, TKey, TQuery>>
  >;
  readonly workflow: CollectionWorkflow<TItem, TKey, TQuery>;
  reload(): Promise<void>;
}

export function useVueCollectionWorkflow<
  TItem,
  TKey extends CollectionKey,
  TQuery,
>(
  options: VueCollectionWorkflowOptions<TItem, TKey, TQuery>,
): VueCollectionWorkflow<TItem, TKey, TQuery> {
  const workflow = createCollectionWorkflow({
    initialQuery: options.initialQuery,
    queryPolicy: options.queryPolicy,
    keyOf: options.keyOf,
    ...(options.initialPage ? { initialPage: options.initialPage } : {}),
  });
  const snapshot = shallowRef(workflow.getSnapshot());
  let started = false;
  let stopped = false;
  let hasLoaded = false;
  let lastLoadedQuery: Readonly<TQuery> | undefined;
  let lastDataQueryKey: CollectionDataQueryKey | undefined;

  function shouldLoad(query: Readonly<TQuery>) {
    if (!hasLoaded) return true;
    if (options.dataQueryKey)
      return !Object.is(options.dataQueryKey(query), lastDataQueryKey);
    return !options.queryPolicy.equals(query, lastLoadedQuery!);
  }

  function invokeLoad(force = false): Promise<void> {
    if (stopped) throw new Error("Vue collection workflow has been disposed.");
    const query = workflow.getSnapshot().query;
    if (!force && !shouldLoad(query)) return Promise.resolve();
    hasLoaded = true;
    lastLoadedQuery = options.queryPolicy.snapshot(query as TQuery);
    lastDataQueryKey = options.dataQueryKey?.(query);
    return options.load(query, workflow);
  }

  const unsubscribeWorkflow = workflow.subscribe((next, change) => {
    snapshot.value = next;
    if (change === "query" && started) void invokeLoad();
  });
  const unbindQuery = options.querySync
    ? bindCollectionQuerySync(workflow, options.querySync)
    : () => undefined;

  onMounted(() => {
    started = true;
    void invokeLoad();
  });
  onScopeDispose(() => {
    if (stopped) return;
    stopped = true;
    unbindQuery();
    unsubscribeWorkflow();
    workflow.dispose();
  });

  return Object.freeze({
    snapshot,
    workflow,
    reload: () => invokeLoad(true),
  });
}
