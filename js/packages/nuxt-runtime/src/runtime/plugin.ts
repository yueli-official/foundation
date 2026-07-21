import {
  createApiClient,
  getApiFailure,
  type ApiClient,
  type ApiFailure,
  type RequestOptions,
  type Transport,
} from "@yueli/http-runtime";
import {
  createError,
  defineNuxtPlugin,
  useRequestEvent,
  useRuntimeConfig,
} from "#app";
import { getHeader, parseCookies, type H3Event } from "h3";

interface RuntimeTargetConfig {
  readonly path: `/${string}`;
}

interface RuntimeConfig {
  readonly defaultTarget: string;
  readonly targets: Readonly<Record<string, RuntimeTargetConfig>>;
}

interface ServerTargetConfig {
  readonly ssr: {
    readonly cookies: readonly string[];
    readonly headers: readonly string[];
  };
}

interface ServerRuntimeConfig {
  readonly targets: Readonly<Record<string, ServerTargetConfig>>;
}

function joinTargetPath(prefix: string, path: string): string {
  if (prefix === "/") {
    return path;
  }
  return `${prefix.replace(/\/$/, "")}${path}`;
}

function statusForFailure(failure: ApiFailure): number {
  if (failure.kind === "remote") {
    return failure.status;
  }
  if (failure.kind === "timeout") {
    return 504;
  }
  if (failure.kind === "network") {
    return 503;
  }
  if (failure.kind === "aborted") {
    return 499;
  }
  return 502;
}

function nuxtClient(client: ApiClient): ApiClient {
  return {
    async request<T>(
      path: `/${string}`,
      options?: RequestOptions<T>,
    ): Promise<T> {
      try {
        return await client.request(path, options);
      } catch (error) {
        const failure = getApiFailure(error);
        if (!failure) {
          throw error;
        }
        throw createError({
          statusCode: statusForFailure(failure),
          statusMessage: failure.code,
          message: failure.code,
          data: { failure },
        });
      }
    },
  };
}

function transportHeaders(
  requestHeaders: Readonly<Record<string, string>>,
  event: H3Event | undefined,
  policy: ServerTargetConfig["ssr"] | undefined,
): Record<string, string> {
  if (!event || !policy) {
    return { ...requestHeaders };
  }

  const forwarded: Record<string, string> = {};
  for (const name of policy.headers) {
    const value = getHeader(event, name);
    if (value !== undefined) {
      forwarded[name] = value;
    }
  }

  const inboundCookies = parseCookies(event);
  const cookie = policy.cookies
    .filter((name) => inboundCookies[name] !== undefined)
    .map((name) => `${name}=${encodeURIComponent(inboundCookies[name]!)}`)
    .join("; ");
  if (cookie) {
    forwarded.cookie = cookie;
  }
  return { ...forwarded, ...requestHeaders };
}

export default defineNuxtPlugin(() => {
  const runtimeConfig = useRuntimeConfig();
  const config = runtimeConfig.public.yueliRuntime as RuntimeConfig;
  const serverConfig = import.meta.server
    ? (runtimeConfig.yueliRuntime as ServerRuntimeConfig)
    : undefined;
  const requestEvent = import.meta.server ? useRequestEvent() : undefined;
  const clients = new Map<string, ApiClient>();

  const transport: Transport = {
    async send(request) {
      const target = config.targets[request.target];
      if (!target) {
        throw new Error("foundation.nuxt.unknown_target");
      }
      const response = await $fetch.raw<unknown>(
        joinTargetPath(target.path, request.path),
        {
          method: request.method,
          headers: transportHeaders(
            request.headers,
            requestEvent,
            serverConfig?.targets[request.target]?.ssr,
          ),
          body: request.body,
          signal: request.signal,
          credentials: "same-origin",
          retry: 0,
          ignoreResponseError: true,
        },
      );
      const body =
        response.status === 204 || request.method === "HEAD"
          ? null
          : JSON.stringify(response._data);

      return new Response(body, {
        status: response.status,
        statusText: response.statusText,
        headers: response.headers,
      });
    },
  };

  function apiFor(target = config.defaultTarget): ApiClient {
    const existing = clients.get(target);
    if (existing) {
      return existing;
    }
    const client = nuxtClient(createApiClient({ target, transport }));
    clients.set(target, client);
    return client;
  }

  return {
    provide: {
      yueliApi: apiFor,
    },
  };
});
