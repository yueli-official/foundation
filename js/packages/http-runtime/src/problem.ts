import {
  isProblemParams,
  protocolFailure,
  type ApiFailure,
  type ProblemParams,
  type Violation,
} from "./failure";

const codePattern = /^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)+$/;
const pointerPattern = /^(?:\/(?:[^~/]|~[01])*)*$/;
const tracePattern = /^[A-Za-z0-9._:-]+$/;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export async function readTextWithinLimit(
  response: Response,
  limit: number,
): Promise<string | undefined> {
  const declaredLength = Number(response.headers.get("content-length"));
  if (Number.isFinite(declaredLength) && declaredLength > limit) {
    return undefined;
  }
  if (!response.body) {
    return "";
  }

  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let length = 0;

  while (true) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    length += value.byteLength;
    if (length > limit) {
      await reader.cancel();
      return undefined;
    }
    chunks.push(value);
  }

  const body = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }

  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(body);
  } catch {
    return undefined;
  }
}

function parseViolations(value: unknown): Violation[] | undefined {
  if (value === undefined) {
    return [];
  }
  if (!Array.isArray(value) || value.length > 64) {
    return undefined;
  }

  const violations: Violation[] = [];
  for (const item of value) {
    if (
      !isRecord(item) ||
      typeof item.pointer !== "string" ||
      item.pointer.length > 1024 ||
      !pointerPattern.test(item.pointer) ||
      typeof item.code !== "string" ||
      !codePattern.test(item.code) ||
      (item.params !== undefined && !isProblemParams(item.params))
    ) {
      return undefined;
    }
    violations.push({
      pointer: item.pointer,
      code: item.code,
      params: (item.params ?? {}) as ProblemParams,
    });
  }
  return violations;
}

export async function failureFromProblemResponse(
  response: Response,
  errorBodyLimit: number,
): Promise<ApiFailure> {
  const headerTraceId = response.headers.get("x-trace-id") ?? undefined;
  const contentType = response.headers
    .get("content-type")
    ?.split(";", 1)[0]
    ?.trim()
    .toLowerCase();
  if (contentType !== "application/problem+json") {
    return protocolFailure(
      "foundation.problem.invalid_content_type",
      headerTraceId,
    );
  }

  const text = await readTextWithinLimit(response, errorBodyLimit);
  if (text === undefined) {
    return protocolFailure("foundation.problem.body_too_large", headerTraceId);
  }

  let value: unknown;
  try {
    value = JSON.parse(text);
  } catch {
    return protocolFailure("foundation.problem.invalid_body", headerTraceId);
  }

  if (!isRecord(value)) {
    return protocolFailure("foundation.problem.invalid_body", headerTraceId);
  }

  const params = value.params ?? {};
  const violations = parseViolations(value.violations);
  if (
    typeof value.type !== "string" ||
    !value.type.startsWith("https://") ||
    value.type.length > 2048 ||
    !Number.isInteger(value.status) ||
    typeof value.status !== "number" ||
    value.status < 400 ||
    value.status > 599 ||
    typeof value.code !== "string" ||
    !codePattern.test(value.code) ||
    !isProblemParams(params) ||
    violations === undefined ||
    typeof value.traceId !== "string" ||
    value.traceId.length > 128 ||
    !tracePattern.test(value.traceId)
  ) {
    return protocolFailure("foundation.problem.invalid_body", headerTraceId);
  }
  if (value.status !== response.status) {
    return protocolFailure(
      "foundation.problem.status_mismatch",
      headerTraceId ?? value.traceId,
    );
  }
  if (headerTraceId !== undefined && headerTraceId !== value.traceId) {
    return protocolFailure("foundation.problem.trace_mismatch", headerTraceId);
  }

  return {
    kind: "remote",
    status: value.status,
    code: value.code,
    params,
    violations,
    traceId: value.traceId,
    reauth: "not-attempted",
  };
}
