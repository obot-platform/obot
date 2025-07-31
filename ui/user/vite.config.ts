import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	server: {
		port: 5174,
		proxy: {
			'/api': 'http://localhost:8080',
			'/legacy-admin': 'http://localhost:8080',
			'/oauth2': 'http://localhost:8080'
		}
	},
	optimizeDeps: {
		// currently incompatible with dep optimizer
		exclude: ['d3-array', 'd3-scale', 'layerchart']
	},
	plugins: [sveltekit()]
});
