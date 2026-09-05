import { expect, test } from "@playwright/test";

test("compact density is opt-in and preserves comfortable component fallbacks", async ({
  page,
}) => {
  test.skip(
    !process.env.FOUNDATION_UI_BASE_URL ||
      !process.env.FOUNDATION_UI_SAMPLE_PATH,
    "requires an existing consumer with a public comment thread",
  );
  for (const width of [390, 750, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    const response = await page.goto(process.env.FOUNDATION_UI_SAMPLE_PATH!);
    expect(await response!.text()).toContain('data-yueli-density="compact"');
    const thread = page.locator("[data-public-comment-thread]").first();
    await expect(thread).toBeVisible();
    const heading = thread.locator("h2").first();
    const composer = thread.locator("textarea").first();
    await expect(composer).toBeVisible();
    expect(
      await heading.evaluate((node) => getComputedStyle(node).fontSize),
    ).toBe("14px");
    expect((await composer.boundingBox())!.height).toBe(80);
    // The semantic field class is the styling interface used by Nuxt UI fields,
    // including portalled forms. Its compact rule must beat mobile defaults.
    await page.evaluate(() => {
      const field = document.createElement("input");
      field.id = "density-conformance-field";
      field.className = "yueli-field-border text-base/5 md:text-sm";
      field.setAttribute("aria-label", "Density conformance field");
      document.body.append(field);
    });
    const field = page.locator("#density-conformance-field");
    expect(
      await field.evaluate((node) => getComputedStyle(node).fontSize),
    ).toBe("14px");
    await page.evaluate(() =>
      document.documentElement.removeAttribute("data-yueli-density"),
    );
    expect(
      await heading.evaluate((node) => getComputedStyle(node).fontSize),
    ).toBe(width < 640 ? "16px" : "18px");
    expect((await composer.boundingBox())!.height).toBe(120);
    expect(
      await field.evaluate((node) => getComputedStyle(node).fontSize),
    ).toBe(width < 768 ? "16px" : "14px");
    await field.evaluate((node) => node.remove());
  }
});
