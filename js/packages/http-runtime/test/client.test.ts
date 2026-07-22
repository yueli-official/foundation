import {
  createApiClient,
  getApiFailure,
  type ReauthAdapter,
  type RequestOptions,
} from "../src/index";
import { createMemoryTransport } from "../src/testing";
import { describe, expect, test, vi } from "vitest";

describe("ApiClient.request", () => {
  test("returns an ordinary JSON success through the caller decoder", async () => {
    const transport = createMemoryTransport(() =>
      Response.json({ items: [{ id: "article-1" }] }),
    );
    const client = createApiClient({ target: "default", transport });

    const result = await client.request("/api/v1/articles", {
      decode(value) {
        if (
          typeof value !== "object" ||
          value === null ||
          !("items" in value) ||
          !Array.isArray(value.items)
        ) {
          throw new Error("invalid article list");
        }
        return value as { items: Array<{ id: string }> };
      },
    });

    expect(result).toEqual({ items: [{ id: "article-1" }] });
  });

  test("throws a safe remote failure for a Problem Details response", async () => {
    const transport = createMemoryTransport(
      () =>
        new Response(
          JSON.stringify({
            type: "https://docs.yueli.dev/problems/blog.slug_taken",
            status: 409,
            code: "blog.slug_taken",
            params: { slug: "hello" },
            violations: [
              {
                pointer: "/slug",
                code: "validation.unique",
                params: { value: "hello" },
              },
            ],
            traceId: "trace-conflict",
            detail: "internal text must not become the Error message",
          }),
          {
            status: 409,
            headers: {
              "content-type": "application/problem+json",
              "x-trace-id": "trace-conflict",
            },
          },
        ),
    );
    const client = createApiClient({ target: "default", transport });

    let caught: unknown;
    try {
      await client.request("/api/v1/articles");
    } catch (error) {
      caught = error;
    }

    expect({
      message: caught instanceof Error ? caught.message : undefined,
      failure: getApiFailure(caught),
    }).toEqual({
      message: "blog.slug_taken",
      failure: {
        kind: "remote",
        status: 409,
        code: "blog.slug_taken",
        params: { slug: "hello" },
        violations: [
          {
            pointer: "/slug",
            code: "validation.unique",
            params: { value: "hello" },
          },
        ],
        traceId: "trace-conflict",
        reauth: "not-attempted",
      },
    });
  });

  test("allows a bounded provider adapter during an error-format migration", async () => {
    const transport = createMemoryTransport(
      () =>
        new Response(JSON.stringify({ code: "legacy.session_expired" }), {
          status: 401,
          headers: { "content-type": "application/json" },
        }),
    );
    const client = createApiClient({ target: "legacy", transport });

    await expect(
      client.request("/api/account", {
        decodeFailure: async (response) => ({
          kind: "remote",
          status: response.status,
          code: "legacy.session_expired",
          params: {},
          violations: [],
          traceId: "legacy-adapter-test",
          reauth: "not-attempted",
        }),
      }),
    ).rejects.toMatchObject({
      failure: {
        kind: "remote",
        status: 401,
        code: "legacy.session_expired",
      },
    });
  });

  test("rejects an origin-escaping path before transport", async () => {
    const transport = createMemoryTransport(() =>
      Response.json({ unreachable: true }),
    );
    const client = createApiClient({ target: "default", transport });

    let caught: unknown;
    try {
      await client.request("//attacker.example/api" as `/${string}`);
    } catch (error) {
      caught = error;
    }

    expect({
      failure: getApiFailure(caught),
      sentRequests: transport.requests.length,
    }).toEqual({
      failure: {
        kind: "protocol",
        code: "foundation.request.invalid_path",
        reauth: "not-attempted",
      },
      sentRequests: 0,
    });
  });

  test("rejects a runtime-invalid method before transport", async () => {
    const transport = createMemoryTransport(() =>
      Response.json({ unreachable: true }),
    );
    const client = createApiClient({ target: "default", transport });

    await expect(
      client.request("/api/v1/articles", {
        method: "TRACE",
      } as unknown as RequestOptions<unknown>),
    ).rejects.toMatchObject({
      failure: { code: "foundation.request.invalid_method" },
    });
    expect(transport.requests).toHaveLength(0);
  });

  test("constructs one encoded JSON transport request", async () => {
    const transport = createMemoryTransport(async (request) =>
      Response.json({
        path: request.path,
        method: request.method,
        headers: request.headers,
        body: request.body,
      }),
    );
    const client = createApiClient({ target: "commerce", transport });

    const observed = await client.request("/api/v1/orders", {
      method: "POST",
      query: {
        tag: ["warm tech", "featured"],
        preview: true,
        omitted: undefined,
      },
      headers: { "if-match": '"version-1"' },
      body: { sku: "book-1", quantity: 2 },
    });

    expect(observed).toEqual({
      path: "/api/v1/orders?tag=warm+tech&tag=featured&preview=true",
      method: "POST",
      headers: {
        "content-type": "application/json",
        "if-match": '"version-1"',
      },
      body: '{"sku":"book-1","quantity":2}',
    });
  });

  test("rejects caller-controlled authorization before transport", async () => {
    const transport = createMemoryTransport(() =>
      Response.json({ unreachable: true }),
    );
    const client = createApiClient({ target: "default", transport });

    let caught: unknown;
    try {
      await client.request("/api/v1/profile", {
        headers: { authorization: "Bearer attacker-controlled" },
      } as RequestOptions<unknown>);
    } catch (error) {
      caught = error;
    }

    expect({
      failure: getApiFailure(caught),
      sentRequests: transport.requests.length,
    }).toEqual({
      failure: {
        kind: "protocol",
        code: "foundation.request.forbidden_header",
        reauth: "not-attempted",
      },
      sentRequests: 0,
    });
  });

  test("restores once and replays a refreshable GET once", async () => {
    let sendCount = 0;
    const transport = createMemoryTransport(() => {
      sendCount += 1;
      if (sendCount === 1) {
        return new Response(
          JSON.stringify({
            type: "https://docs.yueli.dev/problems/auth.session_expired",
            status: 401,
            code: "auth.session_expired",
            traceId: "trace-stale-session",
          }),
          {
            status: 401,
            headers: { "content-type": "application/problem+json" },
          },
        );
      }
      return Response.json({ account: { id: "account-1" } });
    });
    let restoreCount = 0;
    const reauth: ReauthAdapter = {
      async restore() {
        restoreCount += 1;
        return { kind: "renewed" };
      },
    };
    const client = createApiClient({
      target: "identity",
      transport,
      reauth,
      refreshableAuthCodes: ["auth.session_expired"],
    });

    const result = await client.request("/api/v1/account", {
      auth: "required",
    });

    expect({ result, sendCount, restoreCount }).toEqual({
      result: { account: { id: "account-1" } },
      sendCount: 2,
      restoreCount: 1,
    });
  });

  test("applies one timeout budget even when transport ignores the signal", async () => {
    const transport = createMemoryTransport(async () => {
      await new Promise((resolve) => setTimeout(resolve, 30));
      return Response.json({ tooLate: true });
    });
    const client = createApiClient({ target: "default", transport });

    let caught: unknown;
    try {
      await client.request("/api/v1/slow", { timeoutMs: 5 });
    } catch (error) {
      caught = error;
    }

    expect(getApiFailure(caught)).toEqual({
      kind: "timeout",
      code: "foundation.request.timeout",
      reauth: "not-attempted",
    });
  });

  test("never restores a request declared auth none", async () => {
    const transport = createMemoryTransport(
      () =>
        new Response(
          JSON.stringify({
            type: "https://docs.yueli.dev/problems/auth.session_expired",
            status: 401,
            code: "auth.session_expired",
            traceId: "trace-public-endpoint",
          }),
          {
            status: 401,
            headers: { "content-type": "application/problem+json" },
          },
        ),
    );
    let restoreCount = 0;
    const client = createApiClient({
      target: "identity",
      transport,
      refreshableAuthCodes: ["auth.session_expired"],
      reauth: {
        async restore() {
          restoreCount += 1;
          return { kind: "renewed" };
        },
      },
    });

    let caught: unknown;
    try {
      await client.request("/oauth/discovery", { auth: "none" });
    } catch (error) {
      caught = error;
    }

    expect({ failure: getApiFailure(caught), restoreCount }).toEqual({
      failure: {
        kind: "remote",
        status: 401,
        code: "auth.session_expired",
        params: {},
        violations: [],
        traceId: "trace-public-endpoint",
        reauth: "not-attempted",
      },
      restoreCount: 0,
    });
  });

  test("rejects a JSON success above the target response budget", async () => {
    const transport = createMemoryTransport(() =>
      Response.json({ value: "this response is intentionally too large" }),
    );
    const client = createApiClient({
      target: "default",
      transport,
      successBodyLimit: 16,
    });

    let caught: unknown;
    try {
      await client.request("/api/v1/oversized");
    } catch (error) {
      caught = error;
    }

    expect(getApiFailure(caught)).toEqual({
      kind: "protocol",
      code: "foundation.response.body_too_large",
      reauth: "not-attempted",
    });
  });

  test("shares one restore across concurrent stale-auth requests", async () => {
    const attempts = new Map<string, number>();
    const transport = createMemoryTransport((request) => {
      const attempt = (attempts.get(request.path) ?? 0) + 1;
      attempts.set(request.path, attempt);
      if (attempt === 1) {
        return new Response(
          JSON.stringify({
            type: "https://docs.yueli.dev/problems/auth.session_expired",
            status: 401,
            code: "auth.session_expired",
            traceId: `trace-${request.path.slice(1).replaceAll("/", "-")}`,
          }),
          {
            status: 401,
            headers: { "content-type": "application/problem+json" },
          },
        );
      }
      return Response.json({ path: request.path });
    });
    let releaseRestore!: () => void;
    const restoreGate = new Promise<void>((resolve) => {
      releaseRestore = resolve;
    });
    let restoreCount = 0;
    const client = createApiClient({
      target: "identity",
      transport,
      refreshableAuthCodes: ["auth.session_expired"],
      reauth: {
        async restore() {
          restoreCount += 1;
          await restoreGate;
          return { kind: "renewed" };
        },
      },
    });

    const account = client.request("/api/account", { auth: "required" });
    const profile = client.request("/api/profile", { auth: "required" });
    await vi.waitFor(() => expect(restoreCount).toBe(1));
    releaseRestore();

    await expect(Promise.all([account, profile])).resolves.toEqual([
      { path: "/api/account" },
      { path: "/api/profile" },
    ]);
    expect(restoreCount).toBe(1);
  });

  test("does not restore or replay a mutation by default", async () => {
    const transport = createMemoryTransport(
      () =>
        new Response(
          JSON.stringify({
            type: "https://docs.yueli.dev/problems/auth.session_expired",
            status: 401,
            code: "auth.session_expired",
            traceId: "trace-mutation",
          }),
          {
            status: 401,
            headers: { "content-type": "application/problem+json" },
          },
        ),
    );
    let restoreCount = 0;
    const client = createApiClient({
      target: "commerce",
      transport,
      refreshableAuthCodes: ["auth.session_expired"],
      reauth: {
        async restore() {
          restoreCount += 1;
          return { kind: "renewed" };
        },
      },
    });

    await expect(
      client.request("/api/orders", {
        method: "POST",
        auth: "required",
        body: { sku: "book-1" },
      }),
    ).rejects.toThrow("auth.session_expired");
    expect({ restoreCount, requests: transport.requests.length }).toEqual({
      restoreCount: 0,
      requests: 1,
    });
  });

  test("rejects explicit mutation replay without an idempotency key", async () => {
    const transport = createMemoryTransport(() =>
      Response.json({ unreachable: true }),
    );
    const client = createApiClient({ target: "commerce", transport });

    let caught: unknown;
    try {
      await client.request("/api/orders", {
        method: "POST",
        body: { sku: "book-1" },
        replayAfterReauth: "once",
      });
    } catch (error) {
      caught = error;
    }

    expect({
      failure: getApiFailure(caught),
      requests: transport.requests.length,
    }).toEqual({
      failure: {
        kind: "protocol",
        code: "foundation.request.unsafe_replay",
        reauth: "not-attempted",
      },
      requests: 0,
    });
  });

  test("rejects a Problem body whose status disagrees with HTTP", async () => {
    const transport = createMemoryTransport(
      () =>
        new Response(
          JSON.stringify({
            type: "https://docs.yueli.dev/problems/blog.slug_taken",
            status: 400,
            code: "blog.slug_taken",
            traceId: "trace-status-mismatch",
          }),
          {
            status: 409,
            headers: { "content-type": "application/problem+json" },
          },
        ),
    );
    const client = createApiClient({ target: "default", transport });

    let caught: unknown;
    try {
      await client.request("/api/articles");
    } catch (error) {
      caught = error;
    }

    expect(getApiFailure(caught)).toEqual({
      kind: "protocol",
      code: "foundation.problem.status_mismatch",
      traceId: "trace-status-mismatch",
      reauth: "not-attempted",
    });
  });
});
