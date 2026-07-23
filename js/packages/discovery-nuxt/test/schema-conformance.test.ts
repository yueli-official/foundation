import { readFile } from "node:fs/promises";

import type { AnySchema } from "ajv";
import Ajv2020 from "ajv/dist/2020.js";
import { describe, expect, test } from "vitest";

const contractRoot = new URL(
  "../../../../contracts/discovery/",
  import.meta.url,
);

async function readJSON<T = unknown>(relative: string): Promise<T> {
  return JSON.parse(
    await readFile(new URL(relative, contractRoot), "utf8"),
  ) as T;
}

describe("Discovery v1 JSON schema", () => {
  test("accepts canonical page projections", async () => {
    const diagnostic = await readJSON<AnySchema>("diagnostic.v1.schema.json");
    const schema = await readJSON<AnySchema>("page-projection.v1.schema.json");
    const ajv = new Ajv2020({
      strict: true,
      validateFormats: false,
    });
    ajv.addSchema(diagnostic);
    const validate = ajv.compile(schema);
    for (const name of ["article", "unlisted"]) {
      expect(
        validate(await readJSON(`fixtures/valid/${name}.json`)),
        JSON.stringify(validate.errors),
      ).toBe(true);
    }
  });

  test("rejects an unknown major contract", async () => {
    const diagnostic = await readJSON<AnySchema>("diagnostic.v1.schema.json");
    const schema = await readJSON<AnySchema>("page-projection.v1.schema.json");
    const ajv = new Ajv2020({
      strict: true,
      validateFormats: false,
    });
    ajv.addSchema(diagnostic);
    const validate = ajv.compile(schema);
    expect(
      validate(await readJSON("fixtures/invalid/unknown-version.json")),
    ).toBe(false);
  });
});
