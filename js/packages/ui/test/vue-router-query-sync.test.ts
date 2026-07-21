import { createMemoryHistory, createRouter } from "vue-router";
import { describe, expect, it, vi } from "vitest";
import { createVueRouterCollectionQuerySync } from "../src/collection/vue-router";

async function testRouter(query: Record<string, string> = {}) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/", component: { template: "<div />" } }],
  });
  await router.push({ path: "/", query });
  await router.isReady();
  return router;
}

function queryCodec() {
  return {
    ownedKeys: ["q", "page"],
    parse: (query: Readonly<Record<string, unknown>>) => ({
      q: String(query.q ?? ""),
      page: Number(query.page ?? 1),
    }),
    serialize: (query: { q: string; page: number }) => ({
      q: query.q || undefined,
      page: query.page === 1 ? undefined : String(query.page),
    }),
  };
}

describe("createVueRouterCollectionQuerySync", () => {
  it("preserves unowned query, suppresses local echoes and publishes history navigation", async () => {
    const router = await testRouter({ q: "old", page: "2", preview: "1" });
    const replace = vi.spyOn(router, "replace");
    const sync = createVueRouterCollectionQuerySync({
      router,
      codec: queryCodec(),
    });
    const observed: Array<{ q: string; page: number }> = [];
    const stop = sync.subscribe((query) => observed.push(query));

    expect(sync.read()).toEqual({ q: "old", page: 2 });
    await sync.replace({ q: "new", page: 1 });
    expect(replace).toHaveBeenLastCalledWith({
      path: "/",
      query: { preview: "1", q: "new" },
      hash: "",
    });
    expect(router.currentRoute.value.query).toEqual({ preview: "1", q: "new" });
    expect(observed).toEqual([]);

    await router.push({ query: { q: "history", page: "4", preview: "1" } });
    expect(observed).toEqual([{ q: "history", page: 4 }]);
    stop();
  });

  it("serializes rapid local navigation so the latest query wins", async () => {
    const router = await testRouter();
    const sync = createVueRouterCollectionQuerySync({
      router,
      codec: queryCodec(),
    });

    await Promise.all([
      sync.replace({ q: "old", page: 1 }),
      sync.replace({ q: "latest", page: 1 }),
    ]);
    expect(router.currentRoute.value.query).toEqual({ q: "latest" });
  });
});
