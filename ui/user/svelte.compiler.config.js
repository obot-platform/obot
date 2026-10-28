import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

// Shared with Vite and svelte-check so checking can load compiler options
// without loading the build and browser-testing plugins in vite.config.ts.
export default {
	preprocess: [vitePreprocess()]
};
