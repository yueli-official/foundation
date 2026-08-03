import {
  createError,
  defineEventHandler,
  getRequestHeader,
  getRequestURL,
  readRawBody,
  sendWebResponse,
  type EventHandler,
  type H3Event,
} from "h3";

const DEFAULT_MAX_REQUEST_BODY_BYTES = 1024 * 1024;
const DEFAULT_TIMEOUT_MS = 10_000;

const ALLOWED_METHODS = new Set([
  "DELETE",
  "GET",
  "HEAD",
  "OPTIONS",
  "PATCH",
  "POST",
  "PUT",
]);
const PAYLOAD_METHODS = new Set(["DELETE", "PATCH", "POST", "PUT"]);
const RETRYABLE_METHODS = new Set(["GET", "HEAD"]);

const REQUEST_HEADER_ALLOWLIST = [
  "accept",
  "accept-language",
  "content-type",
  "idempotency-key",
  "if-match",
  "if-none-match",
  "prefer",
] as const;

const RESPONSE_HEADER_ALLOWLIST = [
  "cache-control",
  "content-type",
  "etag",
  "last-modified",
  "retry-after",
  "vary",
  "www-authenticate",
  "x-trace-id",
] as const;

const ASSET_REQUEST_HEADERS = ["range"] as const;
const ASSET_RESPONSE_HEADERS = [
  "accept-ranges",
  "content-disposition",
  "content-length",
  "content-range",
  "location",
] as const;

export interface BffTarget {
  /** An origin only, such as `https://api.example.test`; paths are rejected. */
  origin: string;
  /** Optional fixed path owned by the server, such as `/internal/v1`. */
  pathPrefix?: string;
}

/** Resolve only from private server configuration, never from browser input. */
export type BffTargetResolver = (context: {
  event: H3Event;
  signal: AbortSignal;
}) => BffTarget | Promise<BffTarget>;

export type BffCredential =
  { kind: "anonymous" } | { kind: "bearer"; token: string };

export interface BffCredentialAdapter {
  resolve(context: {
    event: H3Event;
    signal: AbortSignal;
  }): BffCredential | Promise<BffCredential>;
}

export interface CreateBffHandlerOptions {
  /** The exact same-origin route prefix mounted by the Nuxt app. */
  mountPath: string;
  /** Reads the owned downstream from private server configuration. */
  resolveTarget: BffTargetResolver;
  /** Replaces browser credentials with an app-owned downstream credential. */
  credential?: BffCredentialAdapter;
  /** Buffered request-body limit. Streaming uploads require a dedicated adapter. */
  maxRequestBodyBytes?: number;
  /** One deadline for target resolution, credential resolution, and downstream I/O. */
  timeoutMs?: number;
  /** Adds vetted range/download headers while retaining the streaming response body. */
  profile?: "api" | "asset";
}

interface ResolvedTarget {
  origin: string;
  pathPrefix: string;
}

export function createBffHandler(
  options: CreateBffHandlerOptions,
): EventHandler {
  const mountPath = validateConfiguredPath(
    options.mountPath,
    "mountPath",
    false,
  );
  const maxRequestBodyBytes = validatePositiveInteger(
    options.maxRequestBodyBytes ?? DEFAULT_MAX_REQUEST_BODY_BYTES,
    "maxRequestBodyBytes",
  );
  const timeoutMs = validatePositiveInteger(
    options.timeoutMs ?? DEFAULT_TIMEOUT_MS,
    "timeoutMs",
  );

  return defineEventHandler(async (event) => {
    const requestURL = getRequestURL(event);
    const rawRequestPath =
      (
        event.node.req.originalUrl ??
        event.node.req.url ??
        requestURL.pathname
      ).split("?", 1)[0] ?? requestURL.pathname;
    const downstreamPath = extractDownstreamPath(rawRequestPath, mountPath);
    if (!ALLOWED_METHODS.has(event.method)) {
      throw createError({
        statusCode: 405,
        statusMessage: "BFF request method is not allowed",
      });
    }
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    timer.unref?.();

    try {
      const target = await resolvePrivateTarget(
        options.resolveTarget,
        event,
        controller.signal,
      );
      const body = await withAbort(
        readLimitedBody(event, maxRequestBodyBytes),
        controller.signal,
      );
      const headers = copyAllowedRequestHeaders(event, options.profile);
      const credential = await resolveCredential(
        options.credential,
        event,
        controller.signal,
      );
      if (credential.kind === "bearer") {
        headers.set("authorization", `Bearer ${credential.token}`);
      }

      const targetURL = new URL(
        `${target.pathPrefix}${downstreamPath}${requestURL.search}`,
        target.origin,
      );

      const response = await fetchDownstream(
        targetURL,
        {
          body: body ? Uint8Array.from(body).buffer : undefined,
          headers,
          method: event.method,
          redirect: "manual",
          signal: controller.signal,
        },
        event.method,
        controller.signal,
      );

      const responseHeaders = copyAllowedResponseHeaders(
        response.headers,
        options.profile,
      );
      const responseBody =
        event.method === "HEAD" || isBodylessStatus(response.status)
          ? null
          : response.body;
      return await sendWebResponse(
        event,
        new Response(responseBody, {
          headers: responseHeaders,
          status: response.status,
          statusText: response.statusText,
        }),
      );
    } catch (error) {
      if (controller.signal.aborted) {
        throw gatewayError(504);
      }
      throw error;
    } finally {
      clearTimeout(timer);
    }
  });
}

async function fetchDownstream(
  targetURL: URL,
  init: RequestInit,
  method: string,
  signal: AbortSignal,
): Promise<Response> {
  const attempts = RETRYABLE_METHODS.has(method) ? 2 : 1;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      return await fetch(targetURL, init);
    } catch {
      if (signal.aborted) throw gatewayError(504);
      if (attempt === attempts - 1) throw gatewayError(502);
    }
  }
  throw gatewayError(502);
}

async function resolvePrivateTarget(
  resolver: BffTargetResolver,
  event: H3Event,
  signal: AbortSignal,
): Promise<ResolvedTarget> {
  try {
    const target = await withAbort(
      Promise.resolve(resolver({ event, signal })),
      signal,
    );
    const parsed = new URL(target.origin);
    if (
      (parsed.protocol !== "http:" && parsed.protocol !== "https:") ||
      parsed.username ||
      parsed.password ||
      parsed.pathname !== "/" ||
      parsed.search ||
      parsed.hash
    ) {
      throw new TypeError("Invalid downstream origin");
    }
    return {
      origin: parsed.origin,
      pathPrefix: target.pathPrefix
        ? validateConfiguredPath(target.pathPrefix, "pathPrefix", true)
        : "",
    };
  } catch {
    throw createError({
      statusCode: 500,
      statusMessage: "BFF target is not configured",
    });
  }
}

function extractDownstreamPath(pathname: string, mountPath: string): string {
  if (pathname !== mountPath && !pathname.startsWith(`${mountPath}/`)) {
    throw createError({
      statusCode: 404,
      statusMessage: "BFF route not found",
    });
  }
  const path = pathname.slice(mountPath.length) || "/";
  try {
    validateRelativePath(path);
  } catch {
    throw createError({
      statusCode: 400,
      statusMessage: "Invalid BFF request path",
    });
  }
  return path;
}

function validateRelativePath(path: string): void {
  if (!path.startsWith("/") || path.startsWith("//") || path.includes("\\")) {
    throw new TypeError("Invalid path");
  }
  const segments = path.split("/");
  for (let index = 1; index < segments.length; index += 1) {
    const segment = segments[index];
    if (!segment) {
      if (index !== segments.length - 1)
        throw new TypeError("Empty path segment");
      continue;
    }
    let decoded = segment;
    for (let pass = 0; pass < 8; pass += 1) {
      const next = decodeURIComponent(decoded);
      decoded = next;
      if (
        decoded === "." ||
        decoded === ".." ||
        decoded.includes("/") ||
        decoded.includes("\\") ||
        containsControlCharacter(decoded)
      ) {
        throw new TypeError("Unsafe path segment");
      }
      if (!decoded.includes("%")) break;
      if (pass === 7) throw new TypeError("Excessively encoded path segment");
    }
  }
}

function validateConfiguredPath(
  value: string,
  name: string,
  allowRoot: boolean,
): string {
  if (
    typeof value !== "string" ||
    (!allowRoot && value === "/") ||
    value.endsWith("/") ||
    value.includes("?") ||
    value.includes("#")
  ) {
    throw new TypeError(`${name} must be a path without a trailing slash`);
  }
  validateRelativePath(value);
  return value;
}

function validatePositiveInteger(value: number, name: string): number {
  if (!Number.isSafeInteger(value) || value <= 0) {
    throw new TypeError(`${name} must be a positive integer`);
  }
  return value;
}

async function readLimitedBody(
  event: H3Event,
  maxBytes: number,
): Promise<Buffer | undefined> {
  if (!PAYLOAD_METHODS.has(event.method)) return undefined;

  const contentEncoding = getRequestHeader(event, "content-encoding");
  if (contentEncoding && contentEncoding.toLowerCase() !== "identity") {
    throw createError({
      statusCode: 415,
      statusMessage: "Encoded BFF request bodies are not supported",
    });
  }

  const contentLength = getRequestHeader(event, "content-length");
  if (contentLength && /^\d+$/u.test(contentLength)) {
    if (Number(contentLength) > maxBytes) throw payloadTooLarge();
  }

  const body = await readRawBody(event, false);
  if (body && body.byteLength > maxBytes) throw payloadTooLarge();
  return body;
}

function payloadTooLarge(): ReturnType<typeof createError> {
  return createError({
    statusCode: 413,
    statusMessage: "BFF request body is too large",
  });
}

function copyAllowedRequestHeaders(
  event: H3Event,
  profile: CreateBffHandlerOptions["profile"],
): Headers {
  const headers = new Headers();
  const allowlist =
    profile === "asset"
      ? [...REQUEST_HEADER_ALLOWLIST, ...ASSET_REQUEST_HEADERS]
      : REQUEST_HEADER_ALLOWLIST;
  for (const name of allowlist) {
    const value = getRequestHeader(event, name);
    if (value) headers.set(name, value);
  }
  return headers;
}

async function resolveCredential(
  adapter: BffCredentialAdapter | undefined,
  event: H3Event,
  signal: AbortSignal,
): Promise<BffCredential> {
  if (!adapter) return { kind: "anonymous" };
  let credential: BffCredential;
  try {
    credential = await withAbort(
      Promise.resolve(adapter.resolve({ event, signal })),
      signal,
    );
  } catch {
    throw gatewayError(signal.aborted ? 504 : 502);
  }
  if (credential.kind === "anonymous") return credential;
  if (
    credential.kind !== "bearer" ||
    typeof credential.token !== "string" ||
    !credential.token ||
    /\s/u.test(credential.token)
  ) {
    throw gatewayError(502);
  }
  return credential;
}

function copyAllowedResponseHeaders(
  source: Headers,
  profile: CreateBffHandlerOptions["profile"],
): Headers {
  const headers = new Headers();
  const allowlist =
    profile === "asset"
      ? [...RESPONSE_HEADER_ALLOWLIST, ...ASSET_RESPONSE_HEADERS]
      : RESPONSE_HEADER_ALLOWLIST;
  for (const name of allowlist) {
    const value = source.get(name);
    if (value) headers.set(name, value);
  }
  return headers;
}

function isBodylessStatus(status: number): boolean {
  return status === 101 || status === 204 || status === 205 || status === 304;
}

function containsControlCharacter(value: string): boolean {
  for (const character of value) {
    const codePoint = character.codePointAt(0);
    if (codePoint !== undefined && (codePoint <= 31 || codePoint === 127)) {
      return true;
    }
  }
  return false;
}

function gatewayError(statusCode: 502 | 504): ReturnType<typeof createError> {
  return createError({
    statusCode,
    statusMessage: statusCode === 504 ? "Gateway Timeout" : "Bad Gateway",
  });
}

function withAbort<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted)
    return Promise.reject(new DOMException("Aborted", "AbortError"));
  return new Promise<T>((resolve, reject) => {
    const abort = () => reject(new DOMException("Aborted", "AbortError"));
    signal.addEventListener("abort", abort, { once: true });
    promise.then(
      (value) => {
        signal.removeEventListener("abort", abort);
        resolve(value);
      },
      (error: unknown) => {
        signal.removeEventListener("abort", abort);
        reject(error);
      },
    );
  });
}
