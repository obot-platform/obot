import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	server: {
		port: 5174,
		proxy: {
			'/api': 'http://localhost:8080',
			'/admin': 'http://localhost:8080',
			'/oauth2': 'http://localhost:8080'
		},
		allowedHosts: ['04ac-70-184-112-91.ngrok-free.app']
	},
	plugins: [sveltekit()]
});
