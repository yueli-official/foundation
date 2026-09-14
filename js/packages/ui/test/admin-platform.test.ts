import { describe, expect, it } from "vitest";
import {
  adminTerm,
  buildAdminFacetValueTree,
  createAdminCategoryParentOptions,
  createAdminFacetValueParentOptions,
  defineAdminProduct,
  projectAdminCategoryCollection,
  projectAdminProduct,
  projectAdminTagCollection,
} from "../src/admin-platform";

const product = defineAdminProduct({
  id: "test-product",
  brand: { label: "Test", icon: "i-tabler-test-pipe", to: "/" },
  vocabulary: { resource: "作品" },
  modules: [
    {
      id: "dashboard",
      label: "控制台",
      icon: "i-tabler-dashboard",
      to: "/admin",
      match: "exact" as const,
      access: { any: ["manage"] },
    },
    {
      id: "content",
      label: "内容",
      icon: "i-tabler-library",
      access: { any: ["manage", "review"] },
      children: [
        {
          id: "items",
          label: "作品",
          icon: "i-tabler-file",
          to: "/admin/items",
          access: { any: ["manage"] },
        },
        {
          id: "review",
          label: "审核",
          icon: "i-tabler-inbox",
          to: "/admin/review",
          access: { any: ["review"] },
        },
      ],
    },
    {
      id: "settings",
      label: "设置",
      icon: "i-tabler-settings",
      to: "/admin/settings",
      placement: "secondary" as const,
      access: { all: ["settings"], any: ["manage", "owner"] },
    },
  ],
});

describe("admin platform", () => {
  it("projects capability-visible navigation and current context", () => {
    const capabilities = new Set(["manage", "settings"]);
    const result = projectAdminProduct(product, {
      path: "/admin/items/42",
      can: (value) => capabilities.has(value),
    });

    expect(result.navigation.map((item) => item.label)).toEqual([
      "控制台",
      "内容",
    ]);
    expect(result.navigation[1]?.active).toBe(true);
    expect(result.navigation[1]?.children?.map((item) => item.label)).toEqual([
      "作品",
    ]);
    expect(result.navigation[1]?.children?.[0]?.active).toBe(true);
    expect(result.secondaryNavigation.map((item) => item.label)).toEqual([
      "设置",
    ]);
    expect(result.currentLabel).toBe("作品");
    expect(result.searchGroups[0]?.items?.map((item) => item.label)).toEqual([
      "控制台",
      "内容 · 作品",
      "设置",
    ]);
  });

  it("honors exact matching for the dashboard", () => {
    const result = projectAdminProduct(product, {
      path: "/admin/items",
      can: () => true,
    });
    expect(result.navigation[0]?.active).toBe(false);
  });

  it("keeps search-excluded children navigable without listing them in commands", () => {
    const definition = defineAdminProduct({
      ...product,
      modules: product.modules.map((module) =>
        module.id === "content"
          ? {
              ...module,
              children: module.children!.map((child) => ({
                ...child,
                search: child.id !== "review",
              })),
            }
          : module,
      ),
    });
    const result = projectAdminProduct(definition, {
      path: "/admin/review",
      can: () => true,
    });
    expect(result.navigation[1]?.children).toHaveLength(2);
    expect(result.currentLabel).toBe("审核");
    expect(result.searchGroups[0]?.items?.map((item) => item.label)).toEqual([
      "控制台",
      "内容 · 作品",
      "设置",
    ]);
    const reviewOnly = projectAdminProduct(definition, {
      path: "/admin/review",
      can: (capability) => capability === "review",
    });
    expect(reviewOnly.navigation).toHaveLength(1);
    expect(reviewOnly.searchGroups).toEqual([]);
  });

  it("uses product vocabulary with a stable fallback", () => {
    expect(adminTerm(product, "resource", "资源")).toBe("作品");
    expect(adminTerm(product, "submission", "投稿")).toBe("投稿");
  });

  it("rejects ambiguous routes", () => {
    expect(() =>
      defineAdminProduct({
        id: "broken",
        brand: { label: "Broken", icon: "i-tabler-alert", to: "/" },
        modules: [
          {
            id: "one",
            label: "One",
            icon: "i-tabler-1-circle",
            to: "/admin/same",
          },
          {
            id: "two",
            label: "Two",
            icon: "i-tabler-2-circle",
            to: "/admin/same",
          },
        ],
      }),
    ).toThrow(/duplicate module route/u);
  });

  it("projects a category tree with ancestor-preserving filters and branch pagination", () => {
    const categories = [
      {
        id: "root-b",
        parentId: "",
        slug: "root-b",
        name: "Root B",
        status: "active" as const,
        editorialPosition: 2,
      },
      {
        id: "root-a",
        parentId: "",
        slug: "root-a",
        name: "Root A",
        status: "active" as const,
        editorialPosition: 1,
      },
      {
        id: "child",
        parentId: "root-a",
        slug: "child",
        name: "Needle",
        status: "inactive" as const,
        editorialPosition: 1,
      },
      {
        id: "grandchild",
        parentId: "child",
        slug: "grandchild",
        name: "Grandchild",
        status: "active" as const,
        editorialPosition: 1,
      },
    ];

    const first = projectAdminCategoryCollection(categories, {
      page: 1,
      pageSize: 1,
      collapsedIds: new Set(["root-a"]),
    });
    expect(first.visibleRows.map((row) => row.id)).toEqual(["root-a"]);
    expect(first.rootCount).toBe(2);
    expect(first.pageCount).toBe(2);

    const filtered = projectAdminCategoryCollection(categories, {
      search: "needle",
      status: "inactive",
      collapsedIds: new Set(["root-a"]),
    });
    expect(filtered.visibleRows.map((row) => row.id)).toEqual([
      "root-a",
      "child",
    ]);
    expect(filtered.matchCount).toBe(1);
    expect(filtered.filtering).toBe(true);
  });

  it("removes self and descendants from category parent choices", () => {
    const categories = [
      {
        id: "root",
        parentId: "",
        slug: "root",
        name: "Root",
        status: "active" as const,
      },
      {
        id: "child",
        parentId: "root",
        slug: "child",
        name: "Child",
        status: "active" as const,
      },
      {
        id: "other",
        parentId: "",
        slug: "other",
        name: "Other",
        status: "active" as const,
      },
    ];
    expect(
      createAdminCategoryParentOptions(categories, {
        currentId: "root",
        rootLabel: "无父分类",
        rootValue: "__root__",
      }),
    ).toEqual([
      { label: "无父分类", value: "__root__" },
      { label: "Other", value: "other" },
    ]);
  });

  it("projects recursive facet values per facet and keeps malformed rows discoverable", () => {
    const values = [
      {
        id: "game",
        facetId: "topic",
        parentId: "",
        slug: "game",
        name: "游戏",
        status: "active" as const,
        editorialPosition: 1,
      },
      {
        id: "action",
        facetId: "topic",
        parentId: "game",
        slug: "action",
        name: "动作",
        status: "active" as const,
        editorialPosition: 1,
      },
      {
        id: "rogue",
        facetId: "topic",
        parentId: "action",
        slug: "rogue",
        name: "肉鸽",
        status: "active" as const,
        editorialPosition: 1,
      },
      {
        id: "orphan",
        facetId: "topic",
        parentId: "missing",
        slug: "orphan",
        name: "孤立项",
        status: "inactive" as const,
        editorialPosition: 2,
      },
      {
        id: "cross",
        facetId: "topic",
        parentId: "cn",
        slug: "cross",
        name: "跨维度父级",
        status: "active" as const,
        editorialPosition: 3,
      },
      {
        id: "cn",
        facetId: "region",
        parentId: "",
        slug: "cn",
        name: "中国",
        status: "active" as const,
        editorialPosition: 1,
      },
      {
        id: "cycle-a",
        facetId: "topic",
        parentId: "cycle-b",
        slug: "cycle-a",
        name: "循环 A",
        status: "active" as const,
        editorialPosition: 4,
      },
      {
        id: "cycle-b",
        facetId: "topic",
        parentId: "cycle-a",
        slug: "cycle-b",
        name: "循环 B",
        status: "active" as const,
        editorialPosition: 5,
      },
    ];

    const rows = buildAdminFacetValueTree(values, "topic");
    expect(rows.map((row) => row.id)).toEqual([
      "game",
      "action",
      "rogue",
      "orphan",
      "cross",
      "cycle-a",
      "cycle-b",
    ]);
    expect(rows.find((row) => row.id === "rogue")?.pathLabel).toBe(
      "游戏 / 动作 / 肉鸽",
    );
    expect(rows.find((row) => row.id === "rogue")?.depth).toBe(2);
    expect(rows.find((row) => row.id === "cross")?.depth).toBe(0);
    expect(rows.find((row) => row.id === "cycle-b")?.ancestorIds).toContain(
      "cycle-a",
    );
    expect(rows.some((row) => row.facetId === "region")).toBe(false);
  });

  it("keeps facet value parent choices inside the facet and removes descendants", () => {
    const values = [
      {
        id: "root",
        facetId: "topic",
        parentId: "",
        slug: "root",
        name: "Root",
        status: "active" as const,
      },
      {
        id: "child",
        facetId: "topic",
        parentId: "root",
        slug: "child",
        name: "Child",
        status: "active" as const,
      },
      {
        id: "grandchild",
        facetId: "topic",
        parentId: "child",
        slug: "grandchild",
        name: "Grandchild",
        status: "active" as const,
      },
      {
        id: "other",
        facetId: "topic",
        parentId: "",
        slug: "other",
        name: "Other",
        status: "active" as const,
      },
      {
        id: "other-child",
        facetId: "topic",
        parentId: "other",
        slug: "other-child",
        name: "Other Child",
        status: "active" as const,
      },
      {
        id: "region",
        facetId: "region",
        parentId: "",
        slug: "region",
        name: "Region",
        status: "active" as const,
      },
    ];

    expect(
      createAdminFacetValueParentOptions(values, {
        facetId: "topic",
        currentId: "root",
        rootLabel: "顶级筛选项",
        rootValue: "__root__",
      }),
    ).toEqual([
      { label: "顶级筛选项", value: "__root__" },
      { label: "Other", value: "other" },
      { label: "Other / Other Child", value: "other-child" },
    ]);
  });

  it("filters, sorts and paginates tag projections", () => {
    const tags = [
      { id: "beta", name: "Beta", status: "active" as const, usageCount: 2 },
      { id: "alpha", name: "Alpha", status: "active" as const, usageCount: 8 },
      { id: "old", name: "Old", status: "inactive" as const, usageCount: 20 },
    ];
    const result = projectAdminTagCollection(tags, {
      status: "active",
      sort: "usage",
      page: 1,
      pageSize: 1,
    });
    expect(result.rows.map((item) => item.id)).toEqual(["alpha", "beta"]);
    expect(result.visibleRows.map((item) => item.id)).toEqual(["alpha"]);
    expect(result.pageCount).toBe(2);
  });
});
