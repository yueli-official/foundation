import type { LocationQuery, LocationQueryRaw, Router } from "vue-router";
import { onScopeDispose, shallowRef, type Ref } from "vue";
import type { CollectionQuerySync } from "./workflow";
import type {
  RouteQuery,
  RouteQueryCodec,
  RouteQueryValue,
} from "./route-query";

export interface VueRouterCollectionQuerySyncOptions<TQuery> {
  readonly router: Pick<Router, "afterEach" | "currentRoute" | "replace">;
  readonly codec: RouteQueryCodec<TQuery>;
  readonly preserveUnknown?: boolean;
}

export interface VueRouterCollectionQuery<TQuery extends object> {
  readonly query: Readonly<Ref<TQuery>>;
  replace(query: TQuery): Promise<void>;
  update(patch: Partial<TQuery>): Promise<void>;
}

function toRouteQuery(query: LocationQuery): RouteQuery {
  return Object.fromEntries(
    Object.entries(query).map(([key, value]) => [
      key,
      Array.isArray(value)
        ? value.filter((entry): entry is string => entry !== null)
        : value,
    ]),
  );
}

function toLocationValue(value: RouteQueryValue): LocationQueryRaw[string] {
  if (Array.isArray(value)) return [...value];
  return value as string | null | undefined;
}

function locationQueryFingerprint(
  query: LocationQuery | LocationQueryRaw,
): string {
  return JSON.stringify(
    Object.keys(query)
      .sort()
      .map((key) => {
        const value = query[key];
        const normalized = Array.isArray(value)
          ? value.map((entry) => (entry == null ? null : String(entry)))
          : value == null
            ? null
            : String(value);
        return [key, normalized];
      }),
  );
}

export function createVueRouterCollectionQuerySync<TQuery>(
  options: VueRouterCollectionQuerySyncOptions<TQuery>,
): CollectionQuerySync<TQuery> {
  const owned = new Set(options.codec.ownedKeys);
  const localNavigations = new Map<string, number>();
  let replaceQueue = Promise.resolve();

  function consumeLocalNavigation(fingerprint: string): boolean {
    const count = localNavigations.get(fingerprint) ?? 0;
    if (count === 0) return false;
    if (count === 1) localNavigations.delete(fingerprint);
    else localNavigations.set(fingerprint, count - 1);
    return true;
  }

  return {
    read() {
      return options.codec.parse(
        toRouteQuery(options.router.currentRoute.value.query),
      );
    },
    replace(query) {
      const current = options.router.currentRoute.value.query;
      const serialized = options.codec.serialize(query as TQuery);
      const next: LocationQueryRaw = {};

      if (options.preserveUnknown !== false) {
        for (const [key, value] of Object.entries(current)) {
          if (!owned.has(key)) next[key] = value;
        }
      }
      for (const [key, value] of Object.entries(serialized)) {
        if (value !== undefined && value !== null && value !== "")
          next[key] = toLocationValue(value);
      }

      const operation = replaceQueue.then(async () => {
        const fingerprint = locationQueryFingerprint(next);
        localNavigations.set(
          fingerprint,
          (localNavigations.get(fingerprint) ?? 0) + 1,
        );
        try {
          const route = options.router.currentRoute.value;
          await options.router.replace({
            path: route.path || "/",
            query: next,
            hash: route.hash,
          });
        } finally {
          consumeLocalNavigation(fingerprint);
        }
      });
      replaceQueue = operation.catch(() => undefined);
      return operation;
    },
    subscribe(listener) {
      return options.router.afterEach((to) => {
        const fingerprint = locationQueryFingerprint(to.query);
        const local = consumeLocalNavigation(fingerprint);
        if (local) return;
        listener(options.codec.parse(toRouteQuery(to.query)));
      });
    },
  };
}

/** Reactive route-query state for collection screens whose data loader is owned by the host app. */
export function useVueRouterCollectionQuery<TQuery extends object>(
  options: VueRouterCollectionQuerySyncOptions<TQuery>,
): VueRouterCollectionQuery<TQuery> {
  const sync = createVueRouterCollectionQuerySync(options);
  const state = shallowRef(sync.read()) as Ref<TQuery>;
  const stop = sync.subscribe((next) => {
    state.value = next;
  });
  onScopeDispose(stop);

  async function replace(next: TQuery) {
    state.value = next;
    await sync.replace(next);
  }

  return {
    query: state as Readonly<Ref<TQuery>>,
    replace,
    update: (patch) => replace({ ...state.value, ...patch }),
  };
}
