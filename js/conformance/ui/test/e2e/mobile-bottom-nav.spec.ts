import { expect, test } from "@playwright/test";

test("mobile navigation owns SSR space, input visibility and desktop release", async ({
  page,
}) => {
  test.skip(
    !process.env.FOUNDATION_UI_BASE_URL,
    "requires a running consumer of MobileBottomNav",
  );
  for (const width of [390, 750, 768, 1440]) {
    await page.setViewportSize({ width, height: 844 });
    const response = await page.goto("/");
    expect(await response!.text()).toContain("data-mobile-bottom-nav");
    const nav = page.locator("[data-mobile-bottom-nav]");
    const spacer = page.locator("[data-mobile-bottom-spacer]");
    if (width >= 768) {
      await expect(nav).toBeHidden();
      await expect(spacer).toBeHidden();
      expect(
        await page.evaluate(() =>
          getComputedStyle(document.documentElement)
            .getPropertyValue("--yueli-mobile-bottom-space")
            .trim(),
        ),
      ).toBe("0px");
      continue;
    }
    await expect(nav).toBeVisible();
    expect((await spacer.boundingBox())!.height).toBe(
      (await nav.boundingBox())!.height,
    );
    await expect(nav.locator('[aria-current="page"]')).toHaveCount(1);
    const input = page.locator("header input").first();
    await input.focus();
    await expect(nav).toBeHidden();
    expect((await spacer.boundingBox())!.height).toBe(49);
    await input.blur();
    await expect(nav).toBeVisible();
    const feedback = page.locator(".yueli-toast-region");
    await expect(feedback).toBeAttached();
    expect(await feedback.evaluate((el) => getComputedStyle(el).bottom)).toBe(
      "65px",
    );
    await nav.evaluate((el) => el.remove());
    expect(await feedback.evaluate((el) => getComputedStyle(el).bottom)).toBe(
      "16px",
    );
  }
});
