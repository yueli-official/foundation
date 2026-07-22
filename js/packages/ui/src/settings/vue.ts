import { computed, ref, type ComputedRef } from "vue";
import {
  createJsonSettingsSnapshotPolicy,
  createSettingsWorkflow,
  type SettingsSnapshotPolicy,
  type SettingsWorkflow,
} from "./workflow";

export interface VueSettingsWorkflowOptions<T> {
  readonly snapshot: () => T;
  readonly restore: (snapshot: T) => void;
  readonly policy?: SettingsSnapshotPolicy<T>;
}

export interface VueSettingsWorkflow<T> {
  readonly dirty: ComputedRef<boolean>;
  readonly workflow: SettingsWorkflow<T>;
  capture(): void;
  discard(): void;
  baseline(): T;
}

export function useVueSettingsWorkflow<T>(
  options: VueSettingsWorkflowOptions<T>,
): VueSettingsWorkflow<T> {
  const workflow = createSettingsWorkflow({
    initial: options.snapshot(),
    policy: options.policy ?? createJsonSettingsSnapshotPolicy<T>(),
  });
  const revision = ref(workflow.revision());
  const dirty = computed(() => {
    void revision.value;
    return workflow.isDirty(options.snapshot());
  });

  return {
    dirty,
    workflow,
    capture: () => {
      workflow.capture(options.snapshot());
      revision.value = workflow.revision();
    },
    discard: () => options.restore(workflow.discard()),
    baseline: () => workflow.baseline(),
  };
}
