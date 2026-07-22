import { describe, expect, it } from "vitest";
import {
  createJsonSettingsSnapshotPolicy,
  createSettingsWorkflow,
} from "../src/settings/workflow";

describe("settings workflow", () => {
  it("owns cloned baseline, structural dirty, capture and discard semantics", () => {
    const initial = { title: "Initial", links: [{ label: "Home" }] };
    const workflow = createSettingsWorkflow({
      initial,
      policy: createJsonSettingsSnapshotPolicy<typeof initial>(),
    });

    initial.links[0]!.label = "Mutated outside";
    expect(workflow.baseline().links[0]!.label).toBe("Home");
    expect(workflow.isDirty(initial)).toBe(true);

    workflow.capture({ links: [{ label: "Saved" }], title: "Next" });
    expect(workflow.revision()).toBe(1);
    expect(
      workflow.isDirty({ title: "Next", links: [{ label: "Saved" }] }),
    ).toBe(false);

    const discarded = workflow.discard();
    discarded.links[0]!.label = "Local only";
    expect(workflow.baseline().links[0]!.label).toBe("Saved");
  });

  it("rejects non-serializable root snapshots", () => {
    const policy = createJsonSettingsSnapshotPolicy<undefined>();
    expect(() => policy.clone(undefined)).toThrow(/JSON-serializable/u);
  });
});
