import { sveltekit } from '@sveltejs/kit/vite';
import { playwright } from '@vitest/browser-playwright';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
	const env = loadEnv(mode, '.', '');
	const apiTarget = env.VITE_API_TARGET || 'http://localhost:8080';
	const apiToken = env.VITE_API_TOKEN;

	// Fail build if API token is set in production - it would be exposed in the bundle
	if (mode === 'production' && apiToken) {
		throw new Error('VITE_API_TOKEN must not be set for production builds');
	}

	// Configure proxy to add auth header when API token is set
	// This is needed for EventSource requests which don't support custom headers
	const proxyConfig = {
		target: apiTarget,
		changeOrigin: true,
		secure: true,
		// The agent terminal is a websocket under /api, and the proxy ignores
		// upgrade requests unless this is set.
		ws: true,
		headers: apiToken ? { Authorization: `Bearer ${apiToken}` } : undefined
	};

	return {
		server: {
			port: 5174,
			proxy:
				mode === 'test'
					? undefined
					: {
							'/api': proxyConfig,
							'/oauth2': proxyConfig
						}
		},
		plugins: [sveltekit()],
		optimizeDeps: {
			// Only reachable via lazily-imported route nodes, so Vite would otherwise
			// discover them mid-navigation and re-bundle, failing in-flight route
			// imports with a 504.
			include: ['d3', 'd3-time-format', 'date-fns', 'es-toolkit']
		},
		test: {
			projects: [
				{
					// Client-side tests (Svelte components)
					extends: true,
					test: {
						name: 'client',
						// Timeout for browser tests - prevent hanging on element lookups
						testTimeout: 2000,
						browser: {
							enabled: true,
							provider: playwright(),
							screenshotFailures: false,
							instances: [{ browser: 'chromium', viewport: { width: 1280, height: 720 } }]
						},
						include: ['src/**/*.svelte.{test,spec}.{js,ts}'],
						exclude: ['src/lib/server/**', 'src/**/*.ssr.{test,spec}.{js,ts}'],
						setupFiles: ['vitest-browser-svelte', 'src/tests/vitest-setup.ts']
					}
				},
				{
					extends: true,
					test: {
						name: 'server',
						environment: 'node',
						include: ['src/**/*.{test,spec}.{js,ts}'],
						exclude: ['src/**/*.svelte.{test,spec}.{js,ts}', 'src/**/*.ssr.{test,spec}.{js,ts}']
					}
				}
			]
		}
	};
});
