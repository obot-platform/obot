import { defineConfig, devices } from '@playwright/test';
import { fileURLToPath } from 'node:url';

const obotPort = Number(process.env.OBOT_E2E_OBOT_PORT ?? '18080');
const uiPort = Number(process.env.OBOT_E2E_UI_PORT ?? '15174');
const baseURL = process.env.BASE_URL ?? `http://127.0.0.1:${uiPort}`;
const repoRoot = fileURLToPath(new URL('../..', import.meta.url));
const userUIRoot = fileURLToPath(new URL('.', import.meta.url));

export default defineConfig({
	testDir: './e2e',
	timeout: 60_000,
	expect: {
		timeout: 10_000
	},
	fullyParallel: false,
	retries: process.env.CI ? 1 : 0,
	reporter: process.env.CI ? 'github' : 'list',
	use: {
		baseURL,
		trace: 'on-first-retry'
	},
	projects: [
		{
			name: 'chromium',
			use: { ...devices['Desktop Chrome'] }
		}
	],
	webServer: [
		{
			command: [
				`rm -rf e2e/.tmp && mkdir -p e2e/.tmp && cd ${repoRoot} && env`,
				'OBOT_BOOTSTRAP_TOKEN=bootstrap-token',
				'OBOT_SERVER_ENABLE_AUTHENTICATION=true',
				'OBOT_SERVER_FORCE_ENABLE_BOOTSTRAP=true',
				'OBOT_DEV_MODE=true',
				`OBOT_SERVER_HOSTNAME='http://127.0.0.1:${uiPort}'`,
				`OBOT_SERVER_UI_HOSTNAME='http://127.0.0.1:${uiPort}'`,
				`OBOT_SERVER_DSN='sqlite://file:${userUIRoot}/e2e/.tmp/obot.db?_journal=WAL&cache=shared&_busy_timeout=30000'`,
				`OBOT_SERVER_PROVIDER_REGISTRIES='${userUIRoot}/e2e/fixtures/provider-registry'`,
				`go run main.go server --http-listen-port ${obotPort} --dev-mode`
			].join(' '),
			url: `http://127.0.0.1:${obotPort}/api/bootstrap`,
			timeout: 120_000,
			reuseExistingServer: !process.env.CI
		},
		{
			command: `VITE_API_TARGET=http://127.0.0.1:${obotPort} pnpm exec vite dev --host 127.0.0.1 --port ${uiPort}`,
			url: baseURL,
			timeout: 120_000,
			reuseExistingServer: !process.env.CI
		}
	]
});
