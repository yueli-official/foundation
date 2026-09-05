import { defineConfig } from "@playwright/test";

const externalBaseURL = process.env.FOUNDATION_UI_BASE_URL;

export default defineConfig({
  testDir: "./test/e2e",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 45_000,
  expect: { timeout: 10_000 },
  outputDir: "../../test-results/ui-foundation-conformance",
  reporter: "line",
  webServer: externalBaseURL
    ? undefined
    : {
        command: "node .output/server/index.mjs",
        env: { NITRO_PORT: "43135" },
        port: 43135,
        reuseExistingServer: false,
        timeout: 120_000,
      },
  use: {
    baseURL: externalBaseURL || "http://127.0.0.1:43135",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});
