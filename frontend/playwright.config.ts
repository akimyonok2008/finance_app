import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  workers: process.env.CI ? 2 : undefined,
  retries: process.env.CI ? 1 : 0,
  timeout: 180_000,
  expect: { timeout: 15_000 },
  reporter: process.env.CI ? [["line"], ["html", { open: "never" }]] : "line",
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://127.0.0.1:18091",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium-release",
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "firefox-contract",
      grep: /@browser-contract/,
      use: { ...devices["Desktop Firefox"] },
    },
    {
      name: "webkit-contract",
      grep: /@browser-contract/,
      use: { ...devices["Desktop Safari"] },
    },
    {
      name: "mobile-chromium",
      grep: /@browser-contract|@mobile/,
      use: { ...devices["Pixel 7"] },
    },
  ],
});
