import { readFile } from "node:fs/promises";

import { describe, expect, it } from "vitest";

import {
  Claimed,
  Collision,
  CompactURLV1,
  deriveUUID,
  HumanCodeV1,
  IdentifierError,
  keyProfile,
  MaxAllocationAttempts,
  newKey,
  newUUID,
  OpaquePublicV1,
  allocateKey,
  parseKey,
  parseUUID,
  UUIDVersion,
} from "../src/index";

describe("Identifier", () => {
  it("matches the repository key-profile contract", async () => {
    const contractURL = new URL(
      "../../../../contracts/identifier/profiles.v1.json",
      import.meta.url,
    );
    const contract = JSON.parse(await readFile(contractURL, "utf8")) as {
      schemaVersion: number;
      profiles: unknown[];
    };
    expect(contract.schemaVersion).toBe(1);
    expect(contract.profiles).toEqual([
      keyProfile(CompactURLV1),
      keyProfile(HumanCodeV1),
      keyProfile(OpaquePublicV1),
    ]);
  });

  it("issues canonical UUIDv7 values", () => {
    const first = newUUID();
    const second = newUUID();
    expect(first).not.toBe(second);
    expect(parseUUID(first)).toBe(first);
    expect(UUIDVersion(first)).toBe(7);
  });

  it("rejects non-canonical UUID text", () => {
    for (const value of [
      "",
      "019C52F0-0000-7000-8000-000000000001",
      "{019c52f0-0000-7000-8000-000000000001}",
      "019c52f0000070008000000000000001",
    ]) {
      expect(() => parseUUID(value)).toThrowError(IdentifierError);
    }
  });

  it("matches the deterministic UUIDv5 contract", async () => {
    const contractURL = new URL(
      "../../../../contracts/identifier/derive.v1.json",
      import.meta.url,
    );
    const contract = JSON.parse(await readFile(contractURL, "utf8")) as {
      schemaVersion: number;
      algorithm: string;
      vectors: Array<{
        namespace: string;
        canonicalNameUTF8: string;
        identifier: string;
      }>;
    };
    expect(contract.schemaVersion).toBe(1);
    expect(contract.algorithm).toBe("uuid-v5");
    for (const vector of contract.vectors) {
      const name = new TextEncoder().encode(vector.canonicalNameUTF8);
      const first = deriveUUID(vector.namespace, name);
      expect(deriveUUID(vector.namespace, name)).toBe(first);
      expect(UUIDVersion(first)).toBe(5);
      expect(first).toBe(vector.identifier);
    }
  });

  it.each([
    [CompactURLV1, 8, /^[1-9A-HJ-NP-Za-km-z]{8}$/],
    [HumanCodeV1, 10, /^[0-9A-HJKMNP-TV-Z]{10}$/],
    [OpaquePublicV1, 16, /^[0-9A-HJKMNP-TV-Z]{16}$/],
  ] as const)("issues and parses %s", (profile, length, pattern) => {
    const value = newKey(profile);
    expect(value).toHaveLength(length);
    expect(value).toMatch(pattern);
    expect(parseKey(profile, value)).toBe(value);
  });

  it("retries collisions through the shared allocation state machine", async () => {
    let attempts = 0;
    const value = await allocateKey(CompactURLV1, () => {
      attempts += 1;
      return attempts < 3 ? Collision : Claimed;
    });
    expect(value).toMatch(/^[1-9A-HJ-NP-Za-km-z]{8}$/);
    expect(attempts).toBe(3);
  });

  it("bounds collision retries", async () => {
    let attempts = 0;
    await expect(
      allocateKey(CompactURLV1, () => {
        attempts += 1;
        return Collision;
      }),
    ).rejects.toMatchObject({ code: "collision_exhausted" });
    expect(attempts).toBe(MaxAllocationAttempts);
  });
});
