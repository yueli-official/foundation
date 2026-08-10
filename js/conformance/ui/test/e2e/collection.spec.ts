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
  await expect(page.getByRole("combobox", { name: "分类" })).toContainText(
    "设计",
  );
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

test("public admin chrome exposes navigation, page ownership and remote search", async ({
  page,
}) => {
  await page.goto("/");
  await settle(page);

  await expect(page.getByText("Yueli UI", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("navigation", { name: "内容集合" }),
  ).toContainText("内容集合");
  await expect(page.locator("#main-content")).toBeVisible();

  const owner = page.getByRole("button", { name: "按负责人筛选" });
  await owner.click();
  await page.getByPlaceholder("搜索负责人").fill("API");
  await expect(page.getByRole("option", { name: /API Team/ })).toBeVisible();
});

test("two-level admin navigation keeps active ancestors open and exposes searchable leaves", async ({
  page,
}, testInfo) => {
  await page.goto("/?section=footer");
  await settle(page);

  const navigation = page.getByRole("navigation", { name: "内容集合" });
  const settings = navigation.getByRole("button", { name: "站点设置" });
  await expect(settings).toHaveAttribute("aria-expanded", "true");
  const footer = navigation.getByRole("link", { name: "页脚" });
  await expect(footer).toBeVisible();
  await expect(footer).toHaveAttribute("data-active", "");

  await page.locator("[data-admin-sidebar-search]").getByRole("button").click();
  const search = page.getByPlaceholder("搜索页面与操作");
  await search.fill("基础");
  const result = page.getByText("站点设置 · 基础", { exact: true });
  await expect(result).toBeVisible();
  await result.click();
  await expect(page).toHaveURL(/section=site/);
  await expect(settings).toHaveAttribute("aria-expanded", "true");
  await expect(navigation.getByRole("link", { name: "基础" })).toHaveAttribute(
    "data-active",
    "",
  );
  await page.screenshot({
    path: testInfo.outputPath("admin-navigation-expanded.png"),
  });
});

test("collapsed admin navigation exposes direct children in a side popover", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto("/?section=footer");
  await settle(page);

  const sidebar = page.getByRole("complementary");
  await page.getByRole("button", { name: "Collapse sidebar" }).click();
  await expect(sidebar).toHaveAttribute("data-collapsed", "true");
  const settings = sidebar
    .getByRole("navigation")
    .locator('[data-slot="link"]')
    .filter({ hasText: "站点设置" });
  await settings.hover();
  await expect(page.getByRole("link", { name: "页脚" })).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("admin-navigation-collapsed.png"),
  });
});

test("mobile parent triggers expand in place and leaf navigation closes the sidebar", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/?section=footer");
  await settle(page);

  await page.getByRole("button", { name: "Open sidebar" }).click();
  const sidebar = page.getByRole("dialog");
  const settings = sidebar.getByRole("button", { name: "站点设置" });
  await expect(settings).toHaveAttribute("aria-expanded", "true");
  await settings.click();
  await expect(settings).toHaveAttribute("aria-expanded", "false");
  await expect(sidebar).toBeVisible();
  await settings.click();
  await sidebar.getByRole("link", { name: "基础" }).click();
  await expect(page).toHaveURL(/section=site/);
  await expect(
    page.getByRole("button", { name: "Open sidebar" }),
  ).toBeVisible();
  await expect(sidebar).toBeHidden();
  await page.screenshot({
    path: testInfo.outputPath("admin-navigation-mobile.png"),
  });
});

test("public account menu exposes grouped actions from an accessible trigger", async ({
  page,
}) => {
  await page.goto("/");
  await settle(page);

  const trigger = page.getByRole("button", {
    name: "打开 Lin 的用户菜单",
  });
  await trigger.focus();
  await trigger.press("Enter");
  await expect(page.getByRole("menuitem", { name: "工作区" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "外观" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "退出登录" })).toBeVisible();
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

test("bulk actions replace the default toolbar in the same collection position", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 1100, height: 560 });
  await page.goto("/");
  await settle(page);
  const toolbarContainer = page.locator("[data-collection-table-toolbar]");
  const containerBox = await toolbarContainer.boundingBox();
  expect(containerBox).not.toBeNull();
  const defaultToolbar = page.locator("[data-collection-table-default]");
  await expect(defaultToolbar).toBeVisible();
  await page.getByRole("checkbox", { name: "选择 公共内容示例 001" }).click();

  const toolbar = page.getByRole("region", { name: "批量操作" });
  await expect(toolbar).toContainText("已选择");
  await expect(defaultToolbar).toBeHidden();
  const selectedContainerBox = await toolbarContainer.boundingBox();
  expect(selectedContainerBox).not.toBeNull();
  expect(
    Math.abs(selectedContainerBox!.y - containerBox!.y),
  ).toBeLessThanOrEqual(1);
  await expect(
    toolbarContainer.getByRole("region", { name: "批量操作" }),
  ).toBeVisible();
  expect(
    await toolbar.evaluate((element) => getComputedStyle(element).position),
  ).toBe("static");
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

  await page.getByRole("button", { name: "打开 Lin 的用户菜单" }).click();
  await page.getByRole("menuitem", { name: "外观" }).click();
  await page.getByRole("menuitemcheckbox", { name: "深色" }).click();
  await expect
    .poll(() => page.locator("html").getAttribute("class"))
    .toContain("dark");
  await page.keyboard.press("Escape");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("menuitem", { name: "退出登录" })).toBeHidden();
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

  await page
    .locator("#main-content")
    .evaluate((element) => element.scrollTo({ top: 700, behavior: "instant" }));
  const backToTop = page.getByRole("button", { name: "返回顶部" });
  await expect(backToTop).toBeVisible();
  await backToTop.click();
  await expect
    .poll(() =>
      page.locator("#main-content").evaluate((element) => element.scrollTop),
    )
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
