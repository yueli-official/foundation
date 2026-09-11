// Run against the Workspace-managed local Gallery; mutations below are intercepted fixtures.
import { chromium, expect } from "../../../../../gallery/web/node_modules/@playwright/test/index.mjs";
import { mkdir, writeFile } from "node:fs/promises";
const origin = process.env.GALLERY_E2E_URL || "http://192.168.5.7:3007";
const account = process.env.GALLERY_E2E_ACCOUNT_URL || "http://192.168.5.7:3400";
const output = process.env.ACCEPTANCE_OUTPUT || "E:/tmp/yueli-authorization-shared";
await mkdir(output, { recursive: true });
const browser = await chromium.launch();
const report = { fixtureApplications: true, fixtureReviews: true, realIdentity: false };
try {
  const context = await browser.newContext();
  const login = await context.request.post(`${account}/api/v1/auth/login`, { data: {
    email: process.env.E2E_EMAIL || "test@example.com", password: process.env.E2E_PASSWORD || "Yueli-local-development-2026",
  } });
  expect(login.ok()).toBeTruthy();
  await context.request.get(`${origin}/auth/login?return_to=/manage`);
  const page = await context.newPage();
  const errors = []; page.on("pageerror", error => errors.push(error.message));
  let applications = ["approve", "reject"].map(id => ({ id: `fixture-${id}`, subject: "TestA123", role: "content_operator", reason: "统一组件验收：申请参与内容维护。", createdAt: "2026-09-08T01:02:00Z" }));
  await page.route("**/manage/console", async route => {
    const response = await route.fetch(); const data = await response.json();
    await route.fulfill({ response, json: { ...data, applications } });
  });
  const decisions = [];
  await page.route("**/manage/applications/fixture-*/review", async route => {
    const decision = route.request().postDataJSON().decision; decisions.push(decision);
    await new Promise(resolve => setTimeout(resolve, 400));
    applications = applications.filter(item => !route.request().url().includes(item.id));
    await route.fulfill({ status: 200, contentType: "application/json", body: "{}" });
  });
  await page.goto(`${origin}/manage/authorization`, { waitUntil: "networkidle" });
  const rows = page.locator("[data-authorization-application]");
  await expect(rows).toHaveCount(2);
  await expect(rows.first()).toContainText("测试管理员"); report.realIdentity = true;
  await expect(rows.first()).toContainText("申请时间：2026/09/08 09:02");
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 900 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBeTruthy();
    await page.screenshot({ path: `${output}/applications-${width}.png`, fullPage: true });
  }
  const profileLink = rows.first().locator(`a[href="${account}/u/TestA123"]`);
  const [profile] = await Promise.all([context.waitForEvent("page"), profileLink.locator("span").click()]);
  await profile.waitForLoadState("networkidle"); await expect(profile.locator("body")).toContainText("测试管理员"); await profile.close();
  await rows.first().getByRole("button", { name: "批准", exact: true }).click();
  await expect(rows.first().getByRole("button", { name: "拒绝", exact: true })).toBeDisabled();
  await expect(rows).toHaveCount(1);
  await rows.first().getByRole("button", { name: "拒绝", exact: true }).click();
  await expect(rows).toHaveCount(0); expect(decisions).toEqual(["approve", "reject"]);
  await page.getByRole("tab", { name: /用户管理/ }).click();
  await expect(page.getByRole("region", { name: "已授权用户" }).getByText("测试管理员", { exact: true })).toBeVisible();
  for (let index = 0; index < 3; index++) {
    await page.reload({ waitUntil: "networkidle" });
    await expect(page.getByRole("region", { name: "已授权用户" }).getByText("测试管理员", { exact: true })).toBeVisible();
  }
  let fail = true;
  await page.route("**/identity-api/api/v1/users?*", async route => {
    if (fail) await route.fulfill({ status: 503, contentType: "application/json", body: "{}" });
    else await route.continue();
  });
  await page.reload({ waitUntil: "networkidle" });
  await expect(page.getByText("用户资料加载失败", { exact: true })).toBeVisible();
  fail = false; await page.getByRole("button", { name: "重试", exact: true }).click();
  await expect(page.getByRole("region", { name: "已授权用户" }).getByText("测试管理员", { exact: true })).toBeVisible();
  await expect(page.getByText("用户资料加载失败", { exact: true })).toHaveCount(0);
  expect(errors).toEqual([]);
  Object.assign(report, { decisions, profileNavigation: true, responsive: true, refreshes: 3, failureRetry: true, errors });
  console.log(JSON.stringify(report));
} finally {
  await browser.close(); await writeFile(`${output}/report.json`, JSON.stringify(report, null, 2));
}
