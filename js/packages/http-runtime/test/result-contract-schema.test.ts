import { readFile } from "node:fs/promises";

import type { AnySchema } from "ajv";
import Ajv2020 from "ajv/dist/2020.js";
import { describe, expect, test } from "vitest";

const contractRoot = new URL(
  "../../../../contracts/http-result/",
  import.meta.url,
);

async function validator(name: string) {
  const schema = JSON.parse(
    await readFile(new URL(name, contractRoot), "utf8"),
  ) as AnySchema;
  return new Ajv2020({ strict: true }).compile(schema);
}

describe("HTTP result contract schemas", () => {
  test("accepts a product error catalog and rejects undeclared fields", async () => {
    const validate = await validator("error-catalog.v1.schema.json");
    const catalog = {
      schemaVersion: "errors.yueli.dev/catalog/v1",
      namespace: "docs",
      errors: [
        {
          code: "docs.import.compression_unsupported",
          status: 400,
          messageKey: "errors.docs.import.compression_unsupported",
          params: { method: { type: "integer", required: true } },
        },
      ],
    };
    expect(validate(catalog)).toBe(true);
    expect(validate({ ...catalog, data: {} })).toBe(false);
  });

  test("distinguishes raw resource, collection and empty success shapes", async () => {
    const validate = await validator("operations.v1.schema.json");
    expect(
      validate({
        schemaVersion: "http.yueli.dev/operations/v1",
        namespace: "docs",
        operations: [
          {
            id: "docs.collections.create",
            method: "POST",
            path: "/collections",
            success: {
              status: 201,
              kind: "resource",
              schemaRef: "#/Collection",
            },
          },
          {
            id: "docs.collections.list",
            method: "GET",
            path: "/collections",
            success: {
              status: 200,
              kind: "collection",
              schemaRef: "#/Collection",
            },
          },
          {
            id: "docs.collections.delete",
            method: "DELETE",
            path: "/collections/{id}",
            success: { status: 204, kind: "empty" },
          },
        ],
      }),
    ).toBe(true);
  });

  test("keeps OAuth endpoints on their protocol-native failure format", async () => {
    const validate = await validator("operations.v1.schema.json");
    expect(
      validate({
        schemaVersion: "http.yueli.dev/operations/v1",
        namespace: "identity",
        operations: [
          {
            id: "identity.oauth.token",
            method: "POST",
            path: "/oauth2/token",
            failureProtocol: "oauth",
            success: {
              status: 200,
              kind: "resource",
              schemaRef: "#/OAuthTokenResponse",
            },
            errors: ["invalid_request", "invalid_client", "invalid_grant"],
          },
        ],
      }),
    ).toBe(true);
  });
});
