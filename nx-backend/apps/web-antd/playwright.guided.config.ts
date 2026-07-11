import { defineConfig, devices } from 'playwright/test';

export default defineConfig({
  testDir: './e2e',
  outputDir: './test-results/guided-video-workflow',
  timeout: 120_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: 'http://127.0.0.1:4317',
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
    video: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: 'pnpm vite --mode development --host 127.0.0.1 --port 4317',
    url: 'http://127.0.0.1:4317',
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
