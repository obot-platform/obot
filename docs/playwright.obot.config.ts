import { defineConfig } from '@playwright/test';

if (!process.env.OBOT_STORAGE_STATE || !process.env.OBOT_TEST_VMCP_ID) {
  throw new Error('Set OBOT_STORAGE_STATE and OBOT_TEST_VMCP_ID; see README.md#product-smoke-tests.');
}

export default defineConfig({
  testDir: './tests',
  testMatch: 'obot.spec.ts',
  timeout: 90_000,
  workers: 1,
  reporter: [['list'], ['html', { outputFolder: 'playwright-report/obot', open: 'never' }]],
  use: {
    headless: false,
    // Audit timestamp assertions use UTC regardless of the developer's timezone.
    timezoneId: 'UTC',
    baseURL: process.env.OBOT_BASE_URL || 'http://localhost:8080',
    storageState: process.env.OBOT_STORAGE_STATE,
    viewport: { width: 1440, height: 1000 },
    launchOptions: process.env.CHROMIUM_PATH ? { executablePath: process.env.CHROMIUM_PATH } : {},
    screenshot: 'only-on-failure',
    // Product traces can contain session credentials; do not collect them.
    trace: 'off',
  },
});
