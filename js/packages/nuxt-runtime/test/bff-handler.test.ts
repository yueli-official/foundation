import { createServer, request as nodeRequest, type Server } from "node:http";
import type { AddressInfo } from "node:net";

import { createApp, toNodeListener } from "h3";
import { afterEach, describe, expect, it } from "vitest";

import {
  createBffHandler,
  type BffCredentialAdapter,
} from "../src/server/index";

interface RunningServer {
  origin: string;
  server: Server;
}

const servers: Server[] = [];

afterEach(async () => {
  await Promise.all(
    servers.splice(0).map(
      (server) =>
        new Promise<void>((resolve, reject) => {
          server.close((error) => (error ? reject(error) : resolve()));
        }),
    ),
  );
});

async function listen(server: Server): Promise<RunningServer> {
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      server.off("error", reject);
      resolve();
    });
  });
  servers.push(server);
  const address = server.address() as AddressInfo;
  return { origin: `http://127.0.0.1:${address.port}`, server };
}

async function startBff(options: {
  downstreamOrigin: string;
  credential?: BffCredentialAdapter;
  maxRequestBodyBytes?: number;
  timeoutMs?: number;
  profile?: "api" | "asset";
}): Promise<RunningServer> {
  const app = createApp();
  app.use(
    "/api/bff/content",
    createBffHandler({
      mountPath: "/api/bff/content",
      resolveTarget: () => ({
        origin: options.downstreamOrigin,
        pathPrefix: "/internal/v1",
      }),
      credential: options.credential,
      maxRequestBodyBytes: options.maxRequestBodyBytes,
      timeoutMs: options.timeoutMs,
      profile: options.profile,
    }),
  );
  return listen(createServer(toNodeListener(app)));
}

async function rawRequest(
  origin: string,
  path: string,
  options: {
    body?: string;
    headers?: Record<string, string>;
    method?: string;
  } = {},
): Promise<{ body: string; status: number }> {
  const target = new URL(origin);
  return new Promise((resolve, reject) => {
    const request = nodeRequest(
      {
        hostname: target.hostname,
        port: target.port,
        headers: options.headers,
        method: options.method ?? "GET",
        path,
      },
      (response) => {
        const chunks: Buffer[] = [];
        response.on("data", (chunk: Buffer) => chunks.push(chunk));
        response.on("end", () =>
          resolve({
            body: Buffer.concat(chunks).toString("utf8"),
            status: response.statusCode ?? 0,
          }),
        );
      },
    );
    request.once("error", reject);
    request.end(options.body);
  });
}

describe("createBffHandler", () => {
  it("uses the private target, safe path/query, allowlisted headers, and adapter credential", async () => {
    let observed: Record<string, unknown> | undefined;
    const downstream = await listen(
      createServer((request, response) => {
        const chunks: Buffer[] = [];
        request.on("data", (chunk: Buffer) => chunks.push(chunk));
        request.on("end", () => {
          observed = {
            body: Buffer.concat(chunks).toString("utf8"),
            headers: request.headers,
            method: request.method,
            url: request.url,
          };
          response.setHeader("content-type", "application/json");
          response.end(JSON.stringify({ ok: true }));
        });
      }),
    );
    const credential: BffCredentialAdapter = {
      resolve: async () => ({ kind: "bearer", token: "server-token" }),
    };
    const bff = await startBff({
      downstreamOrigin: downstream.origin,
      credential,
    });

    const response = await fetch(
      `${bff.origin}/api/bff/content/items/42?view=full&target=http://evil.test`,
      {
        method: "POST",
        headers: {
          accept: "application/json",
          "accept-language": "zh-CN",
          authorization: "Bearer browser-token",
          cookie: "session=browser-secret",
          "content-type": "application/json",
          forwarded: "for=203.0.113.1",
          "idempotency-key": "idem-1",
          traceparent: "00-browser-controlled",
          "x-forwarded-for": "203.0.113.2",
          "x-not-allowed": "drop-me",
          "x-trace-id": "browser-trace",
        },
        body: JSON.stringify({ name: "public" }),
      },
    );

    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ ok: true });
    expect(observed).toMatchObject({
      body: '{"name":"public"}',
      method: "POST",
      url: "/internal/v1/items/42?view=full&target=http://evil.test",
    });
    const headers = observed?.headers as Record<string, string | undefined>;
    expect(headers.authorization).toBe("Bearer server-token");
    expect(headers.accept).toBe("application/json");
    expect(headers["accept-language"]).toBe("zh-CN");
    expect(headers["content-type"]).toBe("application/json");
    expect(headers["idempotency-key"]).toBe("idem-1");
    expect(headers.cookie).toBeUndefined();
    expect(headers.forwarded).toBeUndefined();
    expect(headers.traceparent).toBeUndefined();
    expect(headers["x-forwarded-for"]).toBeUndefined();
    expect(headers["x-not-allowed"]).toBeUndefined();
    expect(headers["x-trace-id"]).toBeUndefined();
    expect(headers.host).toBe(new URL(downstream.origin).host);
  });

  it("passes Problem Details through but discards downstream cookies and unrelated headers", async () => {
    const problem = {
      type: "https://errors.example.test/not-found",
      title: "Not found",
      status: 404,
      code: "content.not_found",
      traceId: "trace-safe-1",
    };
    const downstream = await listen(
      createServer((_request, response) => {
        response.statusCode = 404;
        response.setHeader("content-type", "application/problem+json");
        response.setHeader("set-cookie", "downstream=secret; HttpOnly");
        response.setHeader("x-internal-target", "private-service");
        response.setHeader("x-trace-id", "trace-safe-1");
        response.end(JSON.stringify(problem));
      }),
    );
    const bff = await startBff({ downstreamOrigin: downstream.origin });

    const response = await fetch(`${bff.origin}/api/bff/content/missing`);

    expect(response.status).toBe(404);
    expect(response.headers.get("content-type")).toContain(
      "application/problem+json",
    );
    expect(response.headers.get("x-trace-id")).toBe("trace-safe-1");
    expect(response.headers.get("set-cookie")).toBeNull();
    expect(response.headers.get("x-internal-target")).toBeNull();
    expect(await response.json()).toEqual(problem);
  });

  it("forwards vetted range metadata only for the asset streaming profile", async () => {
    let observedRange: string | undefined;
    const downstream = await listen(
      createServer((request, response) => {
        observedRange = request.headers.range;
        response.statusCode = 206;
        response.setHeader("content-type", "image/webp");
        response.setHeader("accept-ranges", "bytes");
        response.setHeader("content-range", "bytes 0-3/8");
        response.setHeader("x-storage-bucket", "private-bucket");
        response.end("data");
      }),
    );
    const bff = await startBff({
      downstreamOrigin: downstream.origin,
      profile: "asset",
    });

    const response = await fetch(`${bff.origin}/api/bff/content/image.webp`, {
      headers: { range: "bytes=0-3" },
    });

    expect(observedRange).toBe("bytes=0-3");
    expect(response.status).toBe(206);
    expect(response.headers.get("accept-ranges")).toBe("bytes");
    expect(response.headers.get("content-range")).toBe("bytes 0-3/8");
    expect(response.headers.get("x-storage-bucket")).toBeNull();
    expect(await response.text()).toBe("data");
  });

  it.each([
    "/api/bff/content/%2e%2e/admin",
    "/api/bff/content/a/%2E%2E/admin",
    "/api/bff/content/https:%2F%2Fevil.test/admin",
    "/api/bff/content/a%5Cb",
    "/api/bff/content//admin",
  ])("rejects unsafe downstream path %s", async (path) => {
    let downstreamRequests = 0;
    const downstream = await listen(
      createServer((_request, response) => {
        downstreamRequests += 1;
        response.end("unexpected");
      }),
    );
    const bff = await startBff({ downstreamOrigin: downstream.origin });

    const response = await rawRequest(bff.origin, path);

    expect([400, 404]).toContain(response.status);
    expect(response.body).not.toContain(downstream.origin);
    expect(downstreamRequests).toBe(0);
  });

  it("does not follow downstream redirects", async () => {
    let trapRequests = 0;
    const trap = await listen(
      createServer((_request, response) => {
        trapRequests += 1;
        response.end("trap");
      }),
    );
    const downstream = await listen(
      createServer((_request, response) => {
        response.statusCode = 302;
        response.setHeader("location", `${trap.origin}/private`);
        response.end();
      }),
    );
    const bff = await startBff({ downstreamOrigin: downstream.origin });

    const response = await fetch(`${bff.origin}/api/bff/content/redirect`, {
      redirect: "manual",
    });

    expect(response.status).toBe(302);
    expect(response.headers.get("location")).toBeNull();
    expect(trapRequests).toBe(0);
  });

  it("rejects oversized request bodies before forwarding them", async () => {
    let downstreamRequests = 0;
    const downstream = await listen(
      createServer((_request, response) => {
        downstreamRequests += 1;
        response.end("unexpected");
      }),
    );
    const bff = await startBff({
      downstreamOrigin: downstream.origin,
      maxRequestBodyBytes: 8,
    });

    const response = await fetch(`${bff.origin}/api/bff/content/items`, {
      method: "POST",
      headers: { "content-type": "text/plain" },
      body: "123456789",
    });

    expect(response.status).toBe(413);
    expect(downstreamRequests).toBe(0);
  });

  it("rejects unsupported methods and encoded request bodies", async () => {
    let downstreamRequests = 0;
    const downstream = await listen(
      createServer((_request, response) => {
        downstreamRequests += 1;
        response.end("unexpected");
      }),
    );
    const bff = await startBff({ downstreamOrigin: downstream.origin });

    const trace = await rawRequest(bff.origin, "/api/bff/content/items", {
      method: "TRACE",
    });
    const encoded = await rawRequest(bff.origin, "/api/bff/content/items", {
      body: "not-really-gzip",
      headers: { "content-encoding": "gzip" },
      method: "POST",
    });

    expect(trace.status).toBe(405);
    expect(encoded.status).toBe(415);
    expect(downstreamRequests).toBe(0);
  });

  it("applies the request deadline to credential resolution", async () => {
    let downstreamRequests = 0;
    const downstream = await listen(
      createServer((_request, response) => {
        downstreamRequests += 1;
        response.end("unexpected");
      }),
    );
    const bff = await startBff({
      downstreamOrigin: downstream.origin,
      credential: { resolve: () => new Promise(() => {}) },
      timeoutMs: 20,
    });

    const response = await fetch(`${bff.origin}/api/bff/content/items`);

    expect(response.status).toBe(504);
    expect(downstreamRequests).toBe(0);
  });

  it("returns an opaque gateway error when private target resolution is invalid", async () => {
    const app = createApp();
    app.use(
      "/api/bff/content",
      createBffHandler({
        mountPath: "/api/bff/content",
        resolveTarget: () => ({
          origin: "http://user:secret@private.example.test/internal?leak=yes",
        }),
      }),
    );
    const bff = await listen(createServer(toNodeListener(app)));

    const response = await fetch(`${bff.origin}/api/bff/content/items`);
    const body = await response.text();

    expect(response.status).toBe(500);
    expect(body).not.toContain("private.example.test");
    expect(body).not.toContain("secret");
  });
});
