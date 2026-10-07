import { defineConfig, devices } from 'playwright/test';

const isWebKit = process.env.ADMIN_LAYOUT_BROWSER === 'webkit';

export default defineConfig({
  testDir: './e2e',
  testMatch: 'admin-mobile-layout.spec.ts',
  outputDir: './test-results/admin-mobile-layout',
  timeout: 90_000,
  expect: { timeout: 10_000 },
  workers: 1,
  use: {
    actionTimeout: 10_000,
    baseURL: 'http://127.0.0.1:4333',
    screenshot: 'off',
    trace: 'retain-on-failure',
    serviceWorkers: 'block',
    channel: isWebKit ? undefined : process.env.PLAYWRIGHT_CHANNEL,
  },
  projects: [
    {
      name: isWebKit ? 'webkit' : 'chromium',
      use: { ...(isWebKit ? devices['iPhone 13'] : devices['Desktop Chrome']) },
    },
  ],
  webServer: {
    command:
      'pnpm vite --mode development --host 127.0.0.1 --port 4333 --strictPort',
    url: 'http://127.0.0.1:4333',
    reuseExistingServer: false,
    timeout: 120_000,
    env: { VITE_DEV_API_TARGET: 'http://127.0.0.1:9' },
  },
});
