import { describe, expect, it } from "vitest";

import {
  PLATFORM_SESSION_COOKIE_PREFIX,
  selectSsrCookies,
} from "../src/runtime/ssr-cookies";

describe("SSR platform session cookie forwarding", () => {
  it("forwards explicit cookies and every isolated product session", () => {
    expect(
      selectSsrCookies(
        {
          rs_session: "legacy",
          "ys_blog-main_0123456789ab": "blog-session",
          "ys_docs-main_89abcdef0123": "docs-session",
          "yt_blog-main_0123456789ab": "transaction",
          private_cookie: "drop-me",
        },
        ["rs_session"],
        [PLATFORM_SESSION_COOKIE_PREFIX],
      ),
    ).toBe(
      "rs_session=legacy; ys_blog-main_0123456789ab=blog-session; " +
        "ys_docs-main_89abcdef0123=docs-session",
    );
  });

  it("does not treat the OIDC transaction namespace as an API credential", () => {
    expect(
      selectSsrCookies(
        { "yt_blog-main_0123456789ab": "transaction" },
        [],
        [PLATFORM_SESSION_COOKIE_PREFIX],
      ),
    ).toBe("");
  });
});
