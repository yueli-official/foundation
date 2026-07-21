export type ProblemScalar = string | number | boolean;
export type ProblemParam = ProblemScalar | readonly ProblemScalar[];
export type ProblemParams = Readonly<Record<string, ProblemParam>>;
export type ReauthState =
  "not-attempted" | "renewed" | "redirected" | "unavailable";

export interface Violation {
  readonly pointer: string;
  readonly code: string;
  readonly params: ProblemParams;
}

export interface RemoteFailure {
  readonly kind: "remote";
  readonly status: number;
  readonly code: string;
  readonly params: ProblemParams;
  readonly violations: readonly Violation[];
  readonly traceId: string;
  readonly reauth: ReauthState;
}

export interface LocalFailure {
  readonly kind: "network" | "timeout" | "aborted" | "protocol";
  readonly code: `foundation.${string}`;
  readonly params?: ProblemParams;
  readonly traceId?: string;
  readonly reauth: ReauthState;
}

export type ApiFailure = RemoteFailure | LocalFailure;

export class ApiFailureError extends Error {
  readonly failure: ApiFailure;

  constructor(failure: ApiFailure) {
    super(failure.code);
    this.name = "ApiFailureError";
    this.failure = failure;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isReauthState(value: unknown): value is ReauthState {
  return (
    value === "not-attempted" ||
    value === "renewed" ||
    value === "redirected" ||
    value === "unavailable"
  );
}

function isProblemParam(value: unknown): value is ProblemParam {
  if (
    typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
  ) {
    return true;
  }
  return (
    Array.isArray(value) &&
    value.every(
      (item) =>
        typeof item === "string" ||
        typeof item === "number" ||
        typeof item === "boolean",
    )
  );
}

export function isProblemParams(value: unknown): value is ProblemParams {
  return isRecord(value) && Object.values(value).every(isProblemParam);
}

export function isApiFailure(value: unknown): value is ApiFailure {
  if (
    !isRecord(value) ||
    typeof value.code !== "string" ||
    !isReauthState(value.reauth)
  ) {
    return false;
  }

  if (value.kind === "remote") {
    return (
      typeof value.status === "number" &&
      isProblemParams(value.params) &&
      Array.isArray(value.violations) &&
      typeof value.traceId === "string"
    );
  }

  return (
    (value.kind === "network" ||
      value.kind === "timeout" ||
      value.kind === "aborted" ||
      value.kind === "protocol") &&
    value.code.startsWith("foundation.") &&
    (value.params === undefined || isProblemParams(value.params)) &&
    (value.traceId === undefined || typeof value.traceId === "string")
  );
}

export function getApiFailure(error: unknown): ApiFailure | undefined {
  if (isApiFailure(error)) {
    return error;
  }
  if (!isRecord(error)) {
    return undefined;
  }
  if (isApiFailure(error.failure)) {
    return error.failure;
  }
  if (isRecord(error.data) && isApiFailure(error.data.failure)) {
    return error.data.failure;
  }
  return undefined;
}

export function protocolFailure(
  code: `foundation.${string}`,
  traceId?: string,
): LocalFailure {
  return {
    kind: "protocol",
    code,
    ...(traceId ? { traceId } : {}),
    reauth: "not-attempted",
  };
}

export function localFailure(
  kind: "network" | "timeout" | "aborted",
  code: `foundation.${string}`,
): LocalFailure {
  return {
    kind,
    code,
    reauth: "not-attempted",
  };
}
