import {
  ApiFailureError,
  localFailure,
  protocolFailure,
  type ApiFailure,
  type ReauthState,
} from "./failure";
import { failureFromProblemResponse, readTextWithinLimit } from "./problem";

export type HttpMethod = "GET" | "HEAD" | "POST" | "PUT" | "PATCH" | "DELETE";

const allowedHttpMethods = new Set<HttpMethod>([
  "GET",
  "HEAD",
  "POST",
  "PUT",
  "PATCH",
  "DELETE",
]);

export type QueryValue =
  | string
  | number
  | boolean
  | null
  | undefined
  | readonly (string | number | boolean)[];

export type JsonValue =
  | string
  | number
  | boolean
  | null
  | { readonly [key: string]: JsonValue }
  | readonly JsonValue[];

export type AllowedCallerHeader =
  "accept-language" | "if-match" | "if-none-match" | "prefer";

export interface RequestOptions<T> {
  method?: HttpMethod;
  query?: Readonly<Record<string, QueryValue>>;
  body?: JsonValue | FormData | URLSearchParams;
  headers?: Readonly<Partial<Record<AllowedCallerHeader, string>>>;
  signal?: AbortSignal;
  timeoutMs?: number;
  auth?: "none" | "optional" | "required";
  replayAfterReauth?: "never" | "once";
  idempotencyKey?: string;
  decode?: (value: unknown) => T;
  /** Transitional/provider adapter for non-Problem error formats. */
  decodeFailure?: FailureDecoder;
}

export type FailureDecoder = (
  response: Response,
  errorBodyLimit: number,
) => Promise<ApiFailure>;

export interface TransportRequest {
  readonly target: string;
  readonly path: string;
  readonly method: HttpMethod;
  readonly headers: Readonly<Record<string, string>>;
  readonly body?: BodyInit;
  readonly signal: AbortSignal;
}

export interface Transport {
  send(request: TransportRequest): Promise<Response>;
}

export interface ApiClient {
  request<T>(path: `/${string}`, options?: RequestOptions<T>): Promise<T>;
}

export interface ReauthAdapter {
  restore(context: {
    readonly auth: "optional" | "required";
    readonly failure: ApiFailure;
    readonly signal: AbortSignal;
  }): Promise<
    { kind: "renewed" } | { kind: "redirected" } | { kind: "unavailable" }
  >;
}

export interface CreateApiClientOptions {
  readonly target: string;
  readonly transport: Transport;
  readonly errorBodyLimit?: number;
  readonly successBodyLimit?: number;
  readonly reauth?: ReauthAdapter;
  readonly refreshableAuthCodes?: readonly string[];
}

function isSafeRelativePath(path: string): boolean {
  const hasControlCharacter = Array.from(path).some((character) => {
    const code = character.charCodeAt(0);
    return code <= 31 || code === 127;
  });
  if (
    !path.startsWith("/") ||
    path.startsWith("//") ||
    path.includes("\\") ||
    path.includes("?") ||
    path.includes("#") ||
    path.includes("://") ||
    hasControlCharacter
  ) {
    return false;
  }

  try {
    const decoded = decodeURIComponent(path);
    return (
      !decoded.includes("\\") &&
      !decoded.split("/").some((segment) => segment === "..")
    );
  } catch {
    return false;
  }
}

function appendQuery(
  path: string,
  query: RequestOptions<unknown>["query"],
): string {
  if (!query) {
    return path;
  }

  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined) {
      continue;
    }
    if (Array.isArray(value)) {
      for (const item of value) {
        params.append(key, String(item));
      }
    } else {
      params.append(key, value === null ? "" : String(value));
    }
  }
  const encoded = params.toString();
  return encoded ? `${path}?${encoded}` : path;
}

const allowedCallerHeaders = new Set<AllowedCallerHeader>([
  "accept-language",
  "if-match",
  "if-none-match",
  "prefer",
]);

const forbiddenCallerHeaders = new Set([
  "authorization",
  "baggage",
  "connection",
  "content-length",
  "content-type",
  "cookie",
  "forwarded",
  "host",
  "idempotency-key",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "traceparent",
  "tracestate",
  "trailer",
  "transfer-encoding",
  "upgrade",
  "x-trace-id",
]);

function callerHeaderFailure(
  headers: RequestOptions<unknown>["headers"],
): `foundation.${string}` | undefined {
  for (const name of Object.keys(headers ?? {})) {
    const normalized = name.toLowerCase();
    if (
      forbiddenCallerHeaders.has(normalized) ||
      normalized.startsWith("x-forwarded-") ||
      normalized.startsWith("proxy-")
    ) {
      return "foundation.request.forbidden_header";
    }
    if (!allowedCallerHeaders.has(normalized as AllowedCallerHeader)) {
      return "foundation.request.header_not_allowed";
    }
  }
  return undefined;
}

function prepareBodyAndHeaders(options: RequestOptions<unknown>): {
  body?: BodyInit;
  headers: Record<string, string>;
} {
  const headers = Object.fromEntries(
    Object.entries(options.headers ?? {}).map(([name, value]) => [
      name.toLowerCase(),
      value,
    ]),
  );
  if (options.idempotencyKey) {
    headers["idempotency-key"] = options.idempotencyKey;
  }
  if (options.body === undefined) {
    return { headers };
  }
  if (
    options.body instanceof FormData ||
    options.body instanceof URLSearchParams
  ) {
    return { body: options.body, headers };
  }

  headers["content-type"] = "application/json";
  return { body: JSON.stringify(options.body), headers };
}

function reauthState(
  failure: ApiFailure,
  reauth: Exclude<ReauthState, "not-attempted">,
): ApiFailure {
  return { ...failure, reauth };
}

function allowsReplay(
  method: HttpMethod,
  options: RequestOptions<unknown>,
): boolean {
  return (
    options.replayAfterReauth === "once" ||
    (options.replayAfterReauth === undefined &&
      (method === "GET" || method === "HEAD"))
  );
}

function isSafeMutationReplay(
  method: HttpMethod,
  options: RequestOptions<unknown>,
  body: BodyInit | undefined,
): boolean {
  if (method === "GET" || method === "HEAD") {
    return true;
  }
  return Boolean(options.idempotencyKey) && !(body instanceof FormData);
}

function awaitWithSignal<T>(
  promise: Promise<T>,
  signal: AbortSignal,
): Promise<T> {
  if (signal.aborted) {
    return Promise.reject(signal.reason);
  }

  return new Promise<T>((resolve, reject) => {
    const onAbort = () => {
      cleanup();
      reject(signal.reason);
    };
    const cleanup = () => signal.removeEventListener("abort", onAbort);
    signal.addEventListener("abort", onAbort, { once: true });
    promise.then(
      (value) => {
        cleanup();
        resolve(value);
      },
      (error: unknown) => {
        cleanup();
        reject(error);
      },
    );
  });
}

export function createApiClient({
  target,
  transport,
  errorBodyLimit = 64 * 1024,
  successBodyLimit,
  reauth,
  refreshableAuthCodes = [],
}: CreateApiClientOptions): ApiClient {
  const refreshableCodes = new Set(refreshableAuthCodes);
  let restoreFlight:
    | Promise<
        { kind: "renewed" } | { kind: "redirected" } | { kind: "unavailable" }
      >
    | undefined;

  async function restoreOnce(context: Parameters<ReauthAdapter["restore"]>[0]) {
    if (!reauth) {
      return { kind: "unavailable" } as const;
    }
    if (!restoreFlight) {
      const current = reauth
        .restore(context)
        .catch(() => ({ kind: "unavailable" }) as const);
      restoreFlight = current;
      void current.finally(() => {
        if (restoreFlight === current) {
          restoreFlight = undefined;
        }
      });
    }
    return restoreFlight;
  }

  return {
    async request<T>(
      path: `/${string}`,
      options: RequestOptions<T> = {},
    ): Promise<T> {
      if (!isSafeRelativePath(path)) {
        throw new ApiFailureError(
          protocolFailure("foundation.request.invalid_path"),
        );
      }
      const headerFailure = callerHeaderFailure(options.headers);
      if (headerFailure) {
        throw new ApiFailureError(protocolFailure(headerFailure));
      }
      if (
        options.timeoutMs !== undefined &&
        (!Number.isFinite(options.timeoutMs) || options.timeoutMs <= 0)
      ) {
        throw new ApiFailureError(
          protocolFailure("foundation.request.invalid_timeout"),
        );
      }

      const { body, headers } = prepareBodyAndHeaders(options);
      const method = options.method ?? "GET";
      if (!allowedHttpMethods.has(method)) {
        throw new ApiFailureError(
          protocolFailure("foundation.request.invalid_method"),
        );
      }
      const replay = allowsReplay(method, options);
      if (replay && !isSafeMutationReplay(method, options, body)) {
        throw new ApiFailureError(
          protocolFailure("foundation.request.unsafe_replay"),
        );
      }
      const controller = new AbortController();
      let timedOut = false;
      const abortFromCaller = () => controller.abort(options.signal?.reason);
      if (options.signal?.aborted) {
        abortFromCaller();
      } else {
        options.signal?.addEventListener("abort", abortFromCaller, {
          once: true,
        });
      }
      const timeout =
        options.timeoutMs === undefined
          ? undefined
          : setTimeout(() => {
              timedOut = true;
              controller.abort(
                new DOMException("Request timed out", "TimeoutError"),
              );
            }, options.timeoutMs);
      const signal = controller.signal;
      const request: TransportRequest = {
        target,
        path: appendQuery(path, options.query),
        method,
        headers,
        body,
        signal,
      };

      try {
        let response = await awaitWithSignal(transport.send(request), signal);

        if (!response.ok) {
          const wwwAuthenticate =
            response.headers.get("www-authenticate") ?? "";
          const decodeFailure =
            options.decodeFailure ?? failureFromProblemResponse;
          let failure = await awaitWithSignal(
            decodeFailure(response, errorBodyLimit),
            signal,
          );
          const refreshable =
            failure.kind === "remote" &&
            failure.status === 401 &&
            (refreshableCodes.has(failure.code) ||
              /\berror\s*=\s*"?invalid_token"?/i.test(wwwAuthenticate));

          if (refreshable && replay && reauth && options.auth !== "none") {
            const restored = await awaitWithSignal(
              restoreOnce({
                auth: options.auth === "required" ? "required" : "optional",
                failure,
                signal,
              }),
              signal,
            );
            if (restored.kind === "renewed") {
              response = await awaitWithSignal(transport.send(request), signal);
              if (!response.ok) {
                failure = reauthState(
                  await awaitWithSignal(
                    decodeFailure(response, errorBodyLimit),
                    signal,
                  ),
                  "renewed",
                );
                throw new ApiFailureError(failure);
              }
            } else {
              throw new ApiFailureError(reauthState(failure, restored.kind));
            }
          } else {
            throw new ApiFailureError(failure);
          }
        }
        if (response.status === 204 || method === "HEAD") {
          return undefined as T;
        }

        let value: unknown;
        try {
          if (successBodyLimit === undefined) {
            value = await awaitWithSignal(response.json(), signal);
          } else {
            const text = await awaitWithSignal(
              readTextWithinLimit(response, successBodyLimit),
              signal,
            );
            if (text === undefined) {
              throw new ApiFailureError(
                protocolFailure("foundation.response.body_too_large"),
              );
            }
            value = JSON.parse(text);
          }
        } catch (error) {
          if (error instanceof ApiFailureError) {
            throw error;
          }
          if (signal.aborted) {
            throw error;
          }
          throw new ApiFailureError(
            protocolFailure("foundation.response.invalid_json"),
          );
        }
        try {
          return options.decode ? options.decode(value) : (value as T);
        } catch {
          throw new ApiFailureError(
            protocolFailure("foundation.response.decode_failed"),
          );
        }
      } catch (error) {
        if (error instanceof ApiFailureError) {
          throw error;
        }
        if (timedOut) {
          throw new ApiFailureError(
            localFailure("timeout", "foundation.request.timeout"),
          );
        }
        if (signal.aborted) {
          throw new ApiFailureError(
            localFailure("aborted", "foundation.request.aborted"),
          );
        }
        throw new ApiFailureError(
          localFailure("network", "foundation.request.network"),
        );
      } finally {
        if (timeout !== undefined) {
          clearTimeout(timeout);
        }
        options.signal?.removeEventListener("abort", abortFromCaller);
      }
    },
  };
}

export {
  getApiFailure,
  isApiFailure,
  type ApiFailure,
  type LocalFailure,
  type ProblemParam,
  type ProblemParams,
  type ProblemScalar,
  type ReauthState,
  type RemoteFailure,
  type Violation,
} from "./failure";
export { readTextWithinLimit } from "./problem";
