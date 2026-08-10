import { describe, expect, it } from "vitest";
import {
  createAdminNavigationSearchItems,
  normalizeAdminNavigation,
} from "../src/admin/navigation";
import type { AdminNavigationItem } from "../src/admin/types";

describe("admin navigation", () => {
  it("promotes an active child to its parent and opens the group initially", () => {
    const source: readonly AdminNavigationItem[] = [
      {
        label: "Site settings",
        icon: "i-tabler-settings",
        type: "trigger",
        children: [
          { label: "Home", to: "/settings/home" },
          { label: "Footer", to: "/settings/footer", active: true },
        ],
      },
    ];

    const normalized = normalizeAdminNavigation(source);

    expect(normalized[0]).toMatchObject({
      label: "Site settings",
      type: "trigger",
      active: true,
      defaultOpen: true,
    });
    expect(normalized[0]?.children?.[1]).toMatchObject({
      label: "Footer",
      active: true,
    });
    expect(source[0]).not.toHaveProperty("active");
    expect(source[0]).not.toHaveProperty("defaultOpen");
  });

  it("preserves an explicit parent open decision", () => {
    const onSelect = () => undefined;
    const normalized = normalizeAdminNavigation([
      {
        label: "Site settings",
        to: "/settings",
        onSelect,
        defaultOpen: false,
        children: [{ label: "Home", to: "/settings/home", active: true }],
      },
    ]);

    expect(normalized[0]).toMatchObject({
      active: true,
      defaultOpen: false,
      type: "trigger",
      slot: "admin-navigation-group",
      to: undefined,
      onSelect: undefined,
    });
  });

  it("projects flat links and contextual child leaves into search items", () => {
    const items: readonly AdminNavigationItem[] = [
      { label: "Dashboard", icon: "i-tabler-dashboard", to: "/manage" },
      {
        label: "Site settings",
        icon: "i-tabler-settings",
        type: "trigger",
        children: [
          { label: "Home", to: "/manage/home?section=home" },
          { label: "Footer", to: "/manage/home?section=footer" },
          { label: "Local only" },
        ],
      },
    ];

    expect(
      createAdminNavigationSearchItems(items, { idPrefix: "docs-page" }),
    ).toEqual([
      {
        id: "docs-page-0",
        label: "Dashboard",
        icon: "i-tabler-dashboard",
        to: "/manage",
        target: undefined,
        disabled: undefined,
      },
      {
        id: "docs-page-1-0",
        label: "Site settings · Home",
        icon: undefined,
        to: "/manage/home?section=home",
        target: undefined,
        disabled: undefined,
      },
      {
        id: "docs-page-1-1",
        label: "Site settings · Footer",
        icon: undefined,
        to: "/manage/home?section=footer",
        target: undefined,
        disabled: undefined,
      },
    ]);
  });

  it("includes a navigable parent only when the caller opts in", () => {
    const items: readonly AdminNavigationItem[] = [
      {
        label: "Settings",
        to: "/settings",
        children: [{ label: "Profile", to: "/settings/profile" }],
      },
    ];

    expect(createAdminNavigationSearchItems(items)).toHaveLength(1);
    expect(
      createAdminNavigationSearchItems(items, { includeParentLinks: true }),
    ).toHaveLength(2);
  });
});
