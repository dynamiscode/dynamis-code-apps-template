import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./scripts/playwright",
  snapshotPathTemplate: "{testDir}/{testFilePath}-snapshots/{arg}{ext}",
  fullyParallel: false,
  workers: 1,
  reporter: "list",
  timeout: 15_000,
  expect: {
    timeout: 5_000,
    toHaveScreenshot: { maxDiffPixelRatio: 0.05 },
  },
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL,
    channel: "chrome",
    locale: "en-US",
    reducedMotion: "reduce",
    viewport: { width: 1280, height: 900 },
    trace: "retain-on-failure",
  },
});
