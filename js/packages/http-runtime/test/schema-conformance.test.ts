import { readdir, readFile } from "node:fs/promises";

import type { AnySchema } from "ajv";
import Ajv2020 from "ajv/dist/2020.js";
import { describe, expect, test } from "vitest";

const contractRoot = new URL(
  "../../../../contracts/http-problem/",
  import.meta.url,
);

async function readJson<T = unknown>(url: URL): Promise<T> {
  return JSON.parse(await readFile(url, "utf8")) as T;
}

async function fixtureFiles(group: "valid" | "invalid"): Promise<URL[]> {
  const directory = new URL(`fixtures/${group}/`, contractRoot);
  const names = (await readdir(directory)).filter((name) =>
    name.endsWith(".json"),
  );
  return names.sort().map((name) => new URL(name, directory));
}

describe("HTTP Problem v1 schema", () => {
  test("accepts every canonical valid fixture", async () => {
    const schema = await readJson<AnySchema>(
      new URL("http-problem.v1.schema.json", contractRoot),
    );
    const validate = new Ajv2020({ strict: true }).compile(schema);
    const fixtures = await fixtureFiles("valid");

    expect(fixtures.length).toBeGreaterThan(0);
    for (const fixture of fixtures) {
      expect(validate(await readJson(fixture)), fixture.pathname).toBe(true);
    }
  });

  test("rejects every canonical invalid fixture", async () => {
    const schema = await readJson<AnySchema>(
      new URL("http-problem.v1.schema.json", contractRoot),
    );
    const validate = new Ajv2020({ strict: true }).compile(schema);
    const fixtures = await fixtureFiles("invalid");

    expect(fixtures.length).toBeGreaterThan(0);
    for (const fixture of fixtures) {
      expect(validate(await readJson(fixture)), fixture.pathname).toBe(false);
    }
  });
});
