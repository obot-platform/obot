import svelteCompilerConfig from './svelte.compiler.config.js';
import { paraglideVitePlugin } from '@inlang/paraglide-js';
import nodeAdapter from '@sveltejs/adapter-node';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { playwright } from '@vitest/browser-playwright';
import { fileURLToPath } from 'node:url';
import { defineConfig, loadEnv } from 'vite';

/** @type {import('@sveltejs/kit').Config}*/
const svelteConfig = {
	...svelteCompilerConfig,
	// adapter-auto only supports some environments, see https://kit.svelte.dev/docs/adapter-auto for a list.
	// If your environment is not supported, or you settled on a specific environment, switch out the adapter.
	// See https://kit.svelte.dev/docs/adapters for more information about adapters.
	adapter:
		process.env.BUILD === 'node'
			? nodeAdapter({
					out: 'build-node'
				})
			: adapter({
					fallback: 'fallback.html'
				})
};

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
		resolve: {
			alias: {
				$lib: fileURLToPath(new URL('./src/lib', import.meta.url))
			}
		},
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
		plugins: [
			paraglideVitePlugin({
				project: './project.inlang',
				outdir: './src/lib/paraglide',
				// Keep in sync with the `i18n:compile` script in package.json
				emitTsDeclarations: true,
				strategy: ['localStorage', 'preferredLanguage', 'baseLocale']
			}),
			tailwindcss(),
			sveltekit(svelteConfig)
		],
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
						// Concurrent browser files can exhaust the short interaction timeout.
						fileParallelism: false,
						// Timeout for browser tests - prevent hanging on element lookups
						testTimeout: 2000,
						browser: {
							enabled: true,
							// Exposes gc() so the setup file can collect each finished test file's iframe.
							provider: playwright({ launchOptions: { args: ['--js-flags=--expose-gc'] } }),
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
