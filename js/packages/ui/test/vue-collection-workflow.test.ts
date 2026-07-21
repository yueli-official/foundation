// @vitest-environment happy-dom

import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";
import {
  createJsonCollectionQueryPolicy,
  createMemoryCollectionQuerySync,
  type CollectionWorkflow,
} from "../src/collection";
import { useVueCollectionWorkflow } from "../src/collection/vue";

interface Query {
  readonly q: string;
  readonly view: "list" | "grid";
}

interface Item {
  readonly id: string;
}

describe("Vue collection workflow Adapter", () => {
  it("owns setup lifecycle, query binding and data-query invalidation without treating view as data", () => {
    const policy = createJsonCollectionQueryPolicy<Query>();
    const sync = createMemoryCollectionQuerySync<Query>(
      { q: "route", view: "list" },
      policy,
    );
    const loads: Query[] = [];
    let workflow!: CollectionWorkflow<Item, string, Query>;
    let reload!: () => void;

    const wrapper = mount(
      defineComponent({
        setup() {
          const binding = useVueCollectionWorkflow({
            initialQuery: { q: "initial", view: "list" } as Query,
            queryPolicy: policy,
            keyOf: (item: Item) => item.id,
            querySync: sync,
            dataQueryKey: (query) => query.q,
            load: async (query) => {
              loads.push({ ...query });
            },
          });
          workflow = binding.workflow;
          reload = binding.reload;
          return () => h("output", binding.snapshot.value.query.q);
        },
      }),
    );

    expect(wrapper.text()).toBe("route");
    expect(loads).toEqual([{ q: "route", view: "list" }]);

    workflow.setQuery({ q: "route", view: "grid" });
    expect(loads).toHaveLength(1);
    expect(wrapper.text()).toBe("route");

    workflow.setQuery({ q: "next", view: "grid" });
    expect(loads).toEqual([
      { q: "route", view: "list" },
      { q: "next", view: "grid" },
    ]);
    expect(sync.read()).toEqual({ q: "next", view: "grid" });

    reload();
    expect(loads).toHaveLength(3);

    wrapper.unmount();
    expect(() => workflow.setQuery({ q: "disposed", view: "list" })).toThrow(
      "disposed",
    );
  });
});
