import { describe, expect, it } from "vitest";
import {
  bindCollectionQuerySync,
  createCollectionRouteQueryCodec,
  createCollectionWorkflow,
  createJsonCollectionQueryPolicy,
  createMemoryCollectionQuerySync,
  type CollectionQuerySync,
} from "../src/collection";

describe("collection query sync", () => {
  it("builds a caller-owned route codec with hostile-value normalization and default omission", () => {
    const codec = createCollectionRouteQueryCodec({
      q: { kind: "string", default: "", maxLength: 8 },
      status: {
        kind: "enum",
        values: ["all", "draft", "published"] as const,
        default: "all",
      },
      page: { kind: "positive-integer", default: 1 },
      size: {
        kind: "positive-integer",
        values: [10, 20, 40] as const,
        default: 20,
      },
      author: { kind: "string", default: "mine", maxLength: 8 },
    });

    expect(codec.ownedKeys).toEqual(["q", "status", "page", "size", "author"]);
    expect(
      codec.parse({
        q: ["  abcdefghijk  ", "ignored"],
        status: "hostile",
        page: "-2",
        size: "40",
      }),
    ).toEqual({
      q: "abcdefgh",
      status: "all",
      page: 1,
      size: 40,
      author: "mine",
    });
    expect(
      codec.serialize({
        q: "",
        status: "all",
        page: 1,
        size: 20,
        author: "mine",
      }),
    ).toEqual({});
    expect(
      codec.serialize({
        q: "needle",
        status: "draft",
        page: 3,
        size: 40,
        author: "owner",
      }),
    ).toEqual({
      q: "needle",
      status: "draft",
      page: "3",
      size: "40",
      author: "owner",
    });
  });

  it("connects the workflow to a memory Adapter in both directions without feedback writes", () => {
    const policy = createJsonCollectionQueryPolicy<{
      readonly q: string;
      readonly page: number;
    }>();
    const sync = createMemoryCollectionQuerySync(
      { q: "route", page: 2 },
      policy,
    );
    const workflow = createCollectionWorkflow({
      initialQuery: { q: "initial", page: 1 },
      queryPolicy: policy,
      keyOf: (item: { id: string }) => item.id,
    });

    const unbind = bindCollectionQuerySync(workflow, sync);
    expect(workflow.getSnapshot().query).toEqual({ q: "route", page: 2 });

    workflow.setQuery({ q: "local", page: 1 });
    expect(sync.read()).toEqual({ q: "local", page: 1 });

    sync.replace({ q: "back", page: 3 });
    expect(workflow.getSnapshot().query).toEqual({ q: "back", page: 3 });
    expect(sync.history()).toHaveLength(3);
    unbind();
  });

  it("defers intermediate route echoes until all local query writes settle", async () => {
    type Query = { readonly q: string };
    const policy = createJsonCollectionQueryPolicy<Query>();
    const listeners = new Set<(query: Query) => void>();
    const pending: Array<{ query: Readonly<Query>; resolve: () => void }> = [];
    let current: Query = { q: "" };
    const sync: CollectionQuerySync<Query> = {
      read: () => current,
      replace(query) {
        return new Promise<void>((resolve) => pending.push({ query, resolve }));
      },
      subscribe(listener) {
        listeners.add(listener);
        return () => listeners.delete(listener);
      },
    };
    const workflow = createCollectionWorkflow({
      initialQuery: current,
      queryPolicy: policy,
      keyOf: (item: { id: string }) => item.id,
    });
    const unbind = bindCollectionQuerySync(workflow, sync);

    workflow.setQuery({ q: "old" });
    workflow.setQuery({ q: "latest" });
    expect(pending).toHaveLength(2);

    const oldWrite = pending.shift()!;
    current = oldWrite.query as Query;
    for (const listener of listeners) listener(current);
    oldWrite.resolve();
    await Promise.resolve();
    expect(workflow.getSnapshot().query).toEqual({ q: "latest" });

    const latestWrite = pending.shift()!;
    current = latestWrite.query as Query;
    for (const listener of listeners) listener(current);
    latestWrite.resolve();
    await Promise.resolve();
    await Promise.resolve();
    expect(workflow.getSnapshot().query).toEqual({ q: "latest" });
    unbind();
  });
});
