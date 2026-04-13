import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
	const env = loadEnv(mode, '.', '');
	const apiTarget = env.VITE_API_TARGET || 'http://localhost:8080';
	const apiToken = env.VITE_API_TOKEN;
	const localK8sHostSuffix = '.svc.cluster.local';

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
		headers: apiToken ? { Authorization: `Bearer ${apiToken}` } : undefined
	};

	return {
		server: {
			port: 5174,
			allowedHosts: mode !== 'production' ? [localK8sHostSuffix] : [],
			proxy: {
				'/api': proxyConfig,
				'/oauth2': proxyConfig
			}
		},
		plugins: [sveltekit()]
	};
});
