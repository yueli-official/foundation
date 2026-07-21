import { describe, expect, it, vi } from "vitest";
import {
  createCollectionWorkflow,
  createJsonCollectionQueryPolicy,
} from "../src/collection";

interface TestQuery {
  readonly q: string;
  readonly page: number;
  readonly filters: Readonly<{ status: string }>;
}

const queryPolicy = createJsonCollectionQueryPolicy<TestQuery>();
const initialQuery: TestQuery = { q: "", page: 1, filters: { status: "all" } };

function createWorkflow() {
  return createCollectionWorkflow({
    initialQuery,
    queryPolicy,
    keyOf: (item: { id: string }) => item.id,
  });
}

describe("createCollectionWorkflow", () => {
  it("ignores stale responses after the query scope changes", () => {
    const workflow = createWorkflow();
    const first = workflow.beginLoad();
    workflow.setQuery({ ...initialQuery, q: "new" });
    const second = workflow.beginLoad();

    expect(
      workflow.resolveLoad(first, { items: [{ id: "old" }], total: 1 }),
    ).toBe(false);
    expect(
      workflow.resolveLoad(second, { items: [{ id: "new" }], total: 1 }),
    ).toBe(true);
    expect(workflow.getSnapshot()).toMatchObject({
      items: [{ id: "new" }],
      total: 1,
      loadState: "ready",
    });
  });

  it("freezes the current query into an all-results selection request", () => {
    const workflow = createWorkflow();
    const token = workflow.beginLoad();
    workflow.resolveLoad(token, {
      items: [{ id: "a" }, { id: "b" }],
      total: 12,
    });

    workflow.selectAllResults();
    workflow.toggleKey("b");

    expect(workflow.getSelectionRequest()).toEqual({
      mode: "query",
      query: initialQuery,
      excludedKeys: ["b"],
    });
    expect(workflow.getSnapshot().selection.count).toBe(11);
  });

  it("clears cross-page selection when the query scope changes", () => {
    const workflow = createWorkflow();
    const token = workflow.beginLoad();
    workflow.resolveLoad(token, {
      items: [{ id: "a" }, { id: "b" }],
      total: 2,
    });
    workflow.togglePage(true);

    expect(workflow.getSnapshot().selection.count).toBe(2);
    workflow.setQuery({ ...initialQuery, filters: { status: "draft" } });
    expect(workflow.getSelectionRequest()).toEqual({ mode: "keys", keys: [] });
  });

  it("deduplicates visible keys and exposes stable page selection state", () => {
    const workflow = createWorkflow();
    const token = workflow.beginLoad();
    workflow.resolveLoad(token, {
      items: [{ id: "a" }, { id: "a" }, { id: "b" }],
      total: 2,
    });

    workflow.toggleKey("a");
    expect(workflow.getSnapshot()).toMatchObject({
      items: [{ id: "a" }, { id: "b" }],
      selectedPageCount: 1,
      isPageSelected: false,
      isPageIndeterminate: true,
    });
  });

  it("keeps loaded items visible while a refresh is pending and surfaces structured issues", () => {
    const workflow = createWorkflow();
    const initialLoad = workflow.beginLoad();
    workflow.resolveLoad(initialLoad, { items: [{ id: "a" }], total: 1 });

    const changes = vi.fn();
    workflow.subscribe(changes);
    const refresh = workflow.beginLoad();
    expect(workflow.getSnapshot().loadState).toBe("refreshing");
    workflow.rejectLoad(refresh, {
      key: "collection.load.failed",
      params: { traceId: "trace-1" },
    });

    expect(workflow.getSnapshot()).toMatchObject({
      items: [{ id: "a" }],
      loadState: "error",
      issue: { key: "collection.load.failed", params: { traceId: "trace-1" } },
    });
    expect(changes).toHaveBeenCalledTimes(2);
  });

  it("rejects malformed result totals at the public Interface", () => {
    const workflow = createWorkflow();
    const token = workflow.beginLoad();
    expect(() =>
      workflow.resolveLoad(token, { items: [{ id: "a" }], total: -1 }),
    ).toThrow(RangeError);
  });
});
