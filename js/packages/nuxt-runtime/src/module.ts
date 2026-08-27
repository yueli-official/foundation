import { addPlugin, createResolver, defineNuxtModule } from "@nuxt/kit";
import {
  PLATFORM_SESSION_COOKIE_PREFIX,
  platformSessionCookieNames,
} from "./runtime/ssr-cookies";

export type SsrForwardHeader = "accept-language" | "user-agent";

export interface NuxtRuntimeSsrPolicy {
  cookies?: readonly string[];
  headers?: readonly SsrForwardHeader[];
}

export interface NuxtRuntimeTargetOptions {
  path: `/${string}`;
  ssr?: NuxtRuntimeSsrPolicy;
}

export interface NuxtRuntimeModuleOptions {
  defaultTarget: string;
  targets: Readonly<Record<string, NuxtRuntimeTargetOptions>>;
}

function isSafeSameOriginPath(path: string): boolean {
  if (
    !path.startsWith("/") ||
    path.startsWith("//") ||
    path.includes("\\") ||
    path.includes("?") ||
    path.includes("#") ||
    path.includes("://")
  ) {
    return false;
  }

  try {
    return !decodeURIComponent(path)
      .split("/")
      .some((segment) => segment === "..");
  } catch {
    return false;
  }
}

const cookieNamePattern = /^[!#$%&'*+\-.^_`|~0-9A-Za-z]+$/;
const allowedSsrHeaders = new Set<SsrForwardHeader>([
  "accept-language",
  "user-agent",
]);

export default defineNuxtModule<NuxtRuntimeModuleOptions>({
  meta: {
    name: "@yueli/nuxt-runtime",
    configKey: "yueliRuntime",
  },
  defaults: {
    defaultTarget: "default",
    targets: {},
  },
  setup(options, nuxt) {
    if (!(options.defaultTarget in options.targets)) {
      throw new Error(
        `@yueli/nuxt-runtime default target is not configured: ${options.defaultTarget}`,
      );
    }
    for (const [name, target] of Object.entries(options.targets)) {
      if (!/^[a-z][a-z0-9-]*$/.test(name)) {
        throw new Error(`@yueli/nuxt-runtime target name is invalid: ${name}`);
      }
      if (!isSafeSameOriginPath(target.path)) {
        throw new Error(`@yueli/nuxt-runtime target path is invalid: ${name}`);
      }
      for (const cookie of target.ssr?.cookies ?? []) {
        if (!cookieNamePattern.test(cookie)) {
          throw new Error(
            `@yueli/nuxt-runtime SSR cookie name is invalid: ${name}`,
          );
        }
      }
      for (const header of target.ssr?.headers ?? []) {
        if (!allowedSsrHeaders.has(header)) {
          throw new Error(
            `@yueli/nuxt-runtime SSR header is not allowlisted: ${name}`,
          );
        }
      }
    }

    const publicTargets = Object.fromEntries(
      Object.entries(options.targets).map(([name, target]) => [
        name,
        { path: target.path },
      ]),
    );
    const publicRuntimeConfig = nuxt.options.runtimeConfig.public as Record<
      string,
      unknown
    >;
    publicRuntimeConfig.yueliRuntime = {
      defaultTarget: options.defaultTarget,
      targets: publicTargets,
    };
    const privateRuntimeConfig = nuxt.options.runtimeConfig as Record<
      string,
      unknown
    >;
    privateRuntimeConfig.yueliRuntime = {
      targets: Object.fromEntries(
        Object.entries(options.targets).map(([name, target]) => [
          name,
          {
            ssr: {
              cookies: platformSessionCookieNames(target.ssr?.cookies ?? []),
              cookiePrefixes: [PLATFORM_SESSION_COOKIE_PREFIX],
              headers: [...(target.ssr?.headers ?? [])],
            },
          },
        ]),
      ),
    };

    const resolver = createResolver(import.meta.url);
    addPlugin(resolver.resolve("./runtime/plugin"));
    nuxt.hook("prepare:types", ({ references }) => {
      references.push({ path: resolver.resolve("./runtime/types.d.ts") });
    });
  },
});
