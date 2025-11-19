import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	server: {
		host: process.env.VITE_SERVER_HOST || '0.0.0.0',
		port: parseInt(process.env.VITE_SERVER_PORT || '5174', 10),
		allowedHosts: [
			'mcp-catalog.emboldened.ai',
			'localhost',
			'.emboldened.ai',
		],
		proxy: {
			'/api': 'http://localhost:8080',
			'/legacy-admin': 'http://localhost:8080',
			'/oauth2': 'http://localhost:8080'
		}
	},
	optimizeDeps: {
		// currently incompatible with dep optimizer
		exclude: ['layerchart']
	},
	plugins: [sveltekit()]
});
