export interface SettingsSnapshotPolicy<T> {
  clone(value: T): T;
  equals(left: T, right: T): boolean;
}

export interface SettingsWorkflowOptions<T> {
  readonly initial: T;
  readonly policy: SettingsSnapshotPolicy<T>;
}

export interface SettingsWorkflow<T> {
  isDirty(current: T): boolean;
  capture(current: T): void;
  discard(): T;
  baseline(): T;
  revision(): number;
}

function serializeJson(value: unknown): string {
  const serialized = JSON.stringify(value);
  if (serialized === undefined) {
    throw new TypeError("Settings snapshots must be JSON-serializable.");
  }
  return serialized;
}

function canonicalize(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalize);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, entry]) => [key, canonicalize(entry)]),
    );
  }
  return value;
}

function fingerprint(value: unknown): string {
  return JSON.stringify(canonicalize(JSON.parse(serializeJson(value))));
}

export function createJsonSettingsSnapshotPolicy<
  T,
>(): SettingsSnapshotPolicy<T> {
  return {
    clone: (value) => JSON.parse(serializeJson(value)) as T,
    equals: (left, right) => fingerprint(left) === fingerprint(right),
  };
}

export function createSettingsWorkflow<T>(
  options: SettingsWorkflowOptions<T>,
): SettingsWorkflow<T> {
  let saved = options.policy.clone(options.initial);
  let currentRevision = 0;

  return {
    isDirty: (current) => !options.policy.equals(current, saved),
    capture(current) {
      saved = options.policy.clone(current);
      currentRevision += 1;
    },
    discard: () => options.policy.clone(saved),
    baseline: () => options.policy.clone(saved),
    revision: () => currentRevision,
  };
}
