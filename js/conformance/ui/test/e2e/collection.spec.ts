import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

async function settle(page: Page) {
  await page.waitForLoadState("networkidle");
  await page.evaluate(() => document.fonts.ready);
}

async function expectNoHorizontalOverflow(page: Page) {
  const overflow = await page.evaluate(() => ({
    viewport: document.documentElement.clientWidth,
    document: document.documentElement.scrollWidth,
    body: document.body.scrollWidth,
  }));
  expect(overflow.document, JSON.stringify(overflow)).toBeLessThanOrEqual(
    overflow.viewport + 1,
  );
  expect(overflow.body, JSON.stringify(overflow)).toBeLessThanOrEqual(
    overflow.viewport + 1,
  );
}

test("mobile search remains one row and filters update the controlled URL", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  await settle(page);

  const search = page.getByPlaceholder("搜索名称、描述或内容 ID");
  const submit = page.getByRole("button", { name: "搜索", exact: true });
  const searchBox = await search.boundingBox();
  const submitBox = await submit.boundingBox();
  expect(searchBox).not.toBeNull();
  expect(submitBox).not.toBeNull();
  expect(Math.abs(searchBox!.y - submitBox!.y)).toBeLessThanOrEqual(2);
  expect(searchBox!.width).toBeGreaterThan(submitBox!.width * 2);

  await page.getByRole("button", { name: "筛选", exact: true }).click();
  await page.getByRole("combobox", { name: "分类" }).click();
  const designOption = page.getByRole("option", { name: "设计", exact: true });
  await designOption.click();
  await expect(designOption).toBeHidden();
  await expect(page).toHaveURL(/category=design/);
  await expect(page.getByText("设计", { exact: true }).first()).toBeVisible();
  await expectNoHorizontalOverflow(page);
  const pageSizeLabel = await page
    .getByText("每页", { exact: true })
    .boundingBox();
  const pageSizeSelect = await page
    .getByRole("combobox", { name: "每页数量" })
    .boundingBox();
  expect(pageSizeLabel).not.toBeNull();
  expect(pageSizeSelect).not.toBeNull();
  expect(Math.abs(pageSizeLabel!.y - pageSizeSelect!.y)).toBeLessThanOrEqual(6);
  await page.screenshot({
    path: testInfo.outputPath("collection-mobile.png"),
    fullPage: true,
  });
});

test("public dashboard chrome exposes caller-owned labelled regions", async ({
  page,
}) => {
  await page.goto("/");
  await settle(page);

  await expect(
    page.getByRole("heading", { level: 1, name: "内容集合" }),
  ).toBeVisible();
  const metrics = page.getByRole("region", { name: "关键指标" });
  await expect(metrics).toContainText("64");
  await expect(metrics).toContainText("已发布");
});

test("route history restores the query and rendered result", async ({
  page,
}) => {
  await page.goto("/");
  await settle(page);
  await page.goto("/?q=content-002");
  await expect(
    page.getByText("公共内容示例 002", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("公共内容示例 001", { exact: true }),
  ).toBeHidden();

  await page.goBack();
  await expect(page).not.toHaveURL(/(?:\?|&)q=/);
  await expect(
    page.getByText("公共内容示例 001", { exact: true }),
  ).toBeVisible();
});

test("keyboard users can submit search and operate bulk selection", async ({
  page,
}) => {
  await page.goto("/");
  await settle(page);

  const search = page.getByPlaceholder("搜索名称、描述或内容 ID");
  await search.focus();
  await search.pressSequentially("content-010");
  await search.press("Enter");
  await expect(page).toHaveURL(/q=content-010/);
  await expect(
    page.getByText("公共内容示例 010", { exact: true }),
  ).toBeVisible();
  await page.evaluate(
    () =>
      new Promise<void>((resolve) => {
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
      }),
  );

  await search.press("ControlOrMeta+A");
  await search.press("Backspace");
  await expect(search).toHaveValue("");
  await search.press("Enter");
  await expect(page).not.toHaveURL(/(?:\?|&)q=/);
  await expect(search).toHaveValue("", { timeout: 3_000 });
  await expect(page.getByText("公共内容示例 001", { exact: true })).toBeVisible(
    { timeout: 3_000 },
  );
  const checkbox = page.getByRole("checkbox", {
    name: "选择 公共内容示例 001",
  });
  await checkbox.focus();
  await checkbox.press("Space");
  await expect(checkbox).toBeChecked();

  const cancel = page
    .getByRole("region", { name: "批量操作" })
    .getByRole("button", { name: "取消" });
  await cancel.focus();
  await cancel.press("Enter");
  await expect(page.getByRole("region", { name: "批量操作" })).toBeHidden();
});

test("bulk actions stay pinned below the page header while scrolling", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 1100, height: 560 });
  await page.goto("/");
  await settle(page);
  await page.getByRole("checkbox", { name: "选择 公共内容示例 001" }).click();

  const toolbar = page.getByRole("region", { name: "批量操作" });
  await expect(toolbar).toContainText("已选择");
  await page.evaluate(() => window.scrollTo({ top: 700, behavior: "instant" }));
  await expect
    .poll(async () => {
      const box = await toolbar.boundingBox();
      return box?.y ?? -1;
    })
    .toBeGreaterThanOrEqual(0);
  const geometry = await page.evaluate(() => {
    const header = document
      .querySelector("body > div header")
      ?.getBoundingClientRect();
    const toolbar = document
      .querySelector('[aria-label="批量操作"]')
      ?.getBoundingClientRect();
    return {
      headerBottom: header?.bottom ?? 0,
      toolbarTop: toolbar?.top ?? -1,
    };
  });
  expect(geometry.toolbarTop).toBeGreaterThanOrEqual(0);
  await expectNoHorizontalOverflow(page);
  await page.screenshot({
    path: testInfo.outputPath("collection-bulk-sticky.png"),
  });
});

test("light and dark collection states have no detectable axe violations", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/");
  await settle(page);

  let results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath("collection-light.png"),
    fullPage: true,
  });

  await page.getByRole("button", { name: "切换颜色模式" }).click();
  await expect
    .poll(() => page.locator("html").getAttribute("class"))
    .toContain("dark");
  results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath("collection-dark.png"),
    fullPage: true,
  });
});

test("public feedback and back-to-top patterns own their runtime behavior", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 560 });
  await page.goto("/");
  await settle(page);

  await page.getByRole("button", { name: "保存更改" }).click();
  await expect(page.getByRole("button", { name: "保存中" })).toBeVisible();
  await expect(page.getByRole("button", { name: "已保存" })).toBeVisible();

  await page.evaluate(() => window.scrollTo({ top: 700, behavior: "instant" }));
  const backToTop = page.getByRole("button", { name: "返回顶部" });
  await expect(backToTop).toBeVisible();
  await backToTop.click();
  await expect
    .poll(() => page.evaluate(() => window.scrollY))
    .toBeLessThanOrEqual(2);
  await expect(page.locator("#main-content")).toBeFocused();
});

test("public settings workflow owns dirty, discard and save lifecycle", async ({
  page,
}) => {
  await page.goto("/");
  const title = page.getByTestId("settings-title");
  await title.scrollIntoViewIfNeeded();
  await title.fill("未保存工作区");

  const dock = page.locator("[data-settings-save-dock]");
  await expect(dock).toBeVisible();
  await expect(dock).toContainText("有未保存的更改");
  await dock.getByRole("button", { name: "放弃" }).click();
  await expect(title).toHaveValue("公共工作区");
  await expect(dock).toBeHidden();

  await title.fill("已保存工作区");
  await dock.getByRole("button", { name: "保存", exact: true }).click();
  await expect(dock).toContainText("正在保存更改");
  await expect(dock).toContainText("更改已保存");
  await expect(dock).toBeHidden({ timeout: 5_000 });
});
