// @vitest-environment happy-dom
import { nextTick, reactive } from "vue";
import { describe, expect, it } from "vitest";
import { useVueSettingsWorkflow } from "../src/settings/vue";

describe("Vue settings workflow Adapter", () => {
  it("tracks a caller form without owning persistence or fields", async () => {
    const form = reactive({ title: "Initial", nested: { enabled: true } });
    const settings = useVueSettingsWorkflow({
      snapshot: () => form,
      restore: (snapshot) => Object.assign(form, snapshot),
    });

    expect(settings.dirty.value).toBe(false);
    form.title = "Draft";
    await nextTick();
    expect(settings.dirty.value).toBe(true);
    settings.discard();
    expect(form.title).toBe("Initial");
    form.nested.enabled = false;
    settings.capture();
    expect(settings.dirty.value).toBe(false);
    form.title = "After capture";
    settings.capture();
    expect(settings.dirty.value).toBe(false);
  });
});
