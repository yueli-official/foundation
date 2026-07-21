import { expect, test } from "@playwright/test";

test("Problem failure remains structured after hydration", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByTestId("result")).toHaveText(
    "Named target raw JSON success",
  );
  await expect(page.getByTestId("hydrated-failure")).toHaveText(
    "blog.slug_taken",
  );
  await expect(page.getByTestId("downstream-context")).toHaveText(
    JSON.stringify({
      acceptLanguage: "en-US",
      authorization: "Bearer conformance-server-token",
      cookie: null,
      source: "ssr",
    }),
  );
});

test("BFF discards browser credentials and downstream cookies", async ({
  request,
}) => {
  const response = await request.get("/api/bff/proxy/echo?source=browser", {
    headers: {
      authorization: "Bearer browser-token",
      cookie: "session=browser-secret; private=drop-me",
      forwarded: "for=203.0.113.1",
      "x-forwarded-for": "203.0.113.2",
    },
  });

  expect(response.status()).toBe(200);
  expect(response.headers()["set-cookie"]).toBeUndefined();
  expect(await response.json()).toEqual({
    acceptLanguage: "*",
    authorization: "Bearer conformance-server-token",
    cookie: null,
    source: "browser",
  });
});

test("SSR forwards only allowlisted context without cross-request leakage", async ({
  browser,
}) => {
  async function renderContext(session: string, language: string) {
    const context = await browser.newContext({
      locale: language,
      extraHTTPHeaders: {
        authorization: "Bearer must-not-forward",
        "x-forwarded-for": "203.0.113.10",
      },
    });
    await context.addCookies([
      { name: "session", value: session, url: "http://127.0.0.1:43125" },
      {
        name: "secret",
        value: `hidden-${session}`,
        url: "http://127.0.0.1:43125",
      },
    ]);
    const page = await context.newPage();
    await page.goto("/");
    const value = JSON.parse(
      (await page.getByTestId("request-context").textContent()) ?? "null",
    );
    await context.close();
    return value;
  }

  const [alpha, beta] = await Promise.all([
    renderContext("alpha", "fr-CA"),
    renderContext("beta", "de-DE"),
  ]);

  expect({ alpha, beta }).toEqual({
    alpha: {
      cookie: "session=alpha",
      acceptLanguage: "fr-CA",
      authorization: null,
      forwardedFor: null,
    },
    beta: {
      cookie: "session=beta",
      acceptLanguage: "de-DE",
      authorization: null,
      forwardedFor: null,
    },
  });
});
