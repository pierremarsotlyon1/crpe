import { defineConfig } from "@playwright/test";

const baseURL = process.env.E2E_WEB_URL || "http://localhost:3000";
if (!["localhost", "127.0.0.1", "[::1]"].includes(new URL(baseURL).hostname))
  throw new Error("Use an isolated local stack for this suite.");
export default defineConfig({
  testDir: "./tests",
  timeout: 120000,
  expect: { timeout: 12000 },
  workers: 1,
  reporter: "list",
  use: {
    baseURL,
    headless: true,
    channel: process.env.E2E_BROWSER_CHANNEL || undefined,
  },
  outputDir: process.env.E2E_OUTPUT_DIR || "/tmp/crpe-browser-tests",
});
