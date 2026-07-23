import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

import {
  assertDiscoveryProjection,
  DiscoveryContractError,
  toDiscoveryHead,
} from "../src/projection";

function fixture(kind: "valid" | "invalid", name: string): unknown {
  const url = new URL(
    `../../../../contracts/discovery/fixtures/${kind}/${name}.json`,
    import.meta.url,
  );
  return JSON.parse(readFileSync(fileURLToPath(url), "utf8")) as unknown;
}

describe("Discovery projection conformance", () => {
  it.each(["article", "unlisted"])("accepts %s", (name) => {
    const value = fixture("valid", name);
    expect(() => assertDiscoveryProjection(value)).not.toThrow();
  });

  it.each(["unknown-version", "noindex-sitemap", "canonical-drift"])(
    "rejects %s",
    (name) => {
      expect(() => assertDiscoveryProjection(fixture("invalid", name))).toThrow(
        DiscoveryContractError,
      );
    },
  );

  it("uses stable head keys and script-safe JSON", () => {
    const value = fixture("valid", "article") as Record<string, unknown>;
    const head = value.head as Record<string, unknown>;
    head.structuredData = [
      {
        id: "malicious",
        json: {
          "@context": "https://schema.org",
          name: "</script><script>alert(1)</script>",
        },
      },
    ];
    const first = toDiscoveryHead(value);
    const second = toDiscoveryHead(value);
    expect(first).toEqual(second);
    expect(first.script[0]?.innerHTML).not.toContain("</script>");
    expect(first.link.filter((link) => link.rel === "canonical")).toHaveLength(
      1,
    );
  });
});
