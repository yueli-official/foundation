// @vitest-environment happy-dom
import { describe, expect, it, vi } from "vitest";
import { bindSettingsBeforeUnload } from "../src/settings/browser";

describe("settings beforeunload Adapter", () => {
  it("blocks only while dirty and cleans up its listener", () => {
    let dirty = false;
    const cleanup = bindSettingsBeforeUnload({
      isDirty: () => dirty,
      target: window,
    });
    const cleanEvent = new Event("beforeunload", { cancelable: true });
    expect(window.dispatchEvent(cleanEvent)).toBe(true);

    dirty = true;
    const dirtyEvent = new Event("beforeunload", { cancelable: true });
    expect(window.dispatchEvent(dirtyEvent)).toBe(false);

    cleanup();
    const removedEvent = new Event("beforeunload", { cancelable: true });
    expect(window.dispatchEvent(removedEvent)).toBe(true);
    vi.restoreAllMocks();
  });
});
