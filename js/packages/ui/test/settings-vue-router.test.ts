import { beforeEach, describe, expect, it, vi } from "vitest";

let leaveGuard: (() => boolean | Promise<boolean>) | undefined;
vi.mock("vue-router", () => ({
  onBeforeRouteLeave: (guard: () => boolean | Promise<boolean>) => {
    leaveGuard = guard;
  },
}));

describe("settings Router Adapter", () => {
  beforeEach(() => {
    leaveGuard = undefined;
  });

  it("delegates dirty confirmation to caller-owned policy", async () => {
    const { useSettingsLeaveGuard } =
      await import("../src/settings/vue-router");
    let dirty = false;
    const confirm = vi.fn().mockResolvedValue(false);
    useSettingsLeaveGuard({ isDirty: () => dirty, confirm });

    await expect(leaveGuard?.()).resolves.toBe(true);
    dirty = true;
    await expect(leaveGuard?.()).resolves.toBe(false);
    expect(confirm).toHaveBeenCalledOnce();
  });
});
