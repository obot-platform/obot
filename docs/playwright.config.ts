import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  testMatch: 'navigation.spec.ts',
  timeout: 60_000,
  workers: 1,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    headless: false,
    baseURL: process.env.DOCS_BASE_URL || 'http://localhost:3000',
    launchOptions: process.env.CHROMIUM_PATH ? { executablePath: process.env.CHROMIUM_PATH } : {},
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  webServer: process.env.DOCS_BASE_URL ? undefined : {
    command: 'npm run preview:pages',
    url: 'http://localhost:3000/next/',
    reuseExistingServer: !process.env.CI,
  },
});
