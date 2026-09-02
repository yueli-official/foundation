import { describe, expect, it } from "vitest";

import { createBrowserUUID } from "../src/runtime/identifiers";

describe("createBrowserUUID", () => {
  it("uses randomUUID in secure browser contexts", () => {
    const expected = "019c0000-0000-4000-8000-000000000001";
    expect(
      createBrowserUUID({
        getRandomValues: (value) => value,
        randomUUID: () => expected,
      }),
    ).toBe(expected);
  });

  it("creates a UUID when randomUUID is unavailable on LAN HTTP", () => {
    expect(
      createBrowserUUID({
        getRandomValues: (value) => {
          if (value instanceof Uint8Array) value.fill(0);
          return value;
        },
      }),
    ).toBe("00000000-0000-4000-8000-000000000000");
  });

  it("keeps the non-crypto fallback within identifier contracts", () => {
    expect(createBrowserUUID(null)).toMatch(
      /^client-[a-z0-9]+-[a-z0-9]{11,}$/u,
    );
  });
});
