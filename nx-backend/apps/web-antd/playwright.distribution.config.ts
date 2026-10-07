import { defineConfig, devices } from 'playwright/test';

export default defineConfig({
  testDir: './e2e',
  testMatch: 'distribution-layout.spec.ts',
  outputDir: './test-results/distribution-layout',
  timeout: 90_000,
  expect: { timeout: 10_000 },
  workers: 1,
  use: {
    actionTimeout: 10_000,
    baseURL: 'http://127.0.0.1:4331',
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
    serviceWorkers: 'block',
    channel: process.env.PLAYWRIGHT_CHANNEL,
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command:
      'pnpm vite --mode development --host 127.0.0.1 --port 4331 --strictPort',
    url: 'http://127.0.0.1:4331',
    reuseExistingServer: false,
    timeout: 120_000,
    // A missed request must never reach a real backend.
    env: { VITE_DEV_API_TARGET: 'http://127.0.0.1:9' },
  },
});
