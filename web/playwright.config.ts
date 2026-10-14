import { defineConfig } from "@playwright/test";
import { tmpdir } from "node:os";
import { join } from "node:path";
export default defineConfig({
  testDir: "tests",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 600000,
  expect: { timeout: 20000 },
  outputDir:
    process.env.STEERLINE_WEB_EVIDENCE ||
    join(tmpdir(), "steerline-web-browser-" + Date.now()),
  reporter: "list",
  use: {
    browserName: "chromium",
    channel: "msedge",
    headless: true,
    viewport: { width: 1440, height: 1000 },
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
});
