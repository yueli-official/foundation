import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./test/e2e",
  use: {
    baseURL: "http://127.0.0.1:43125",
  },
  webServer: {
    command: "node .output/server/index.mjs",
    env: {
      NITRO_PORT: "43125",
      NUXT_CONFORMANCE_DOWNSTREAM_ORIGIN: "http://127.0.0.1:43125",
    },
    port: 43125,
    reuseExistingServer: false,
    timeout: 30_000,
  },
});
