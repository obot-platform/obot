import nodeAdapter from '@sveltejs/adapter-node';
import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config}*/
const config = {
	// Consult https://kit.svelte.dev/docs/integrations#preprocessors
	// for more information about preprocessors
	preprocess: [vitePreprocess()],
	kit: {
		adapter: adapter({
			fallback: 'fallback.html'
		})
	}
};

if (process.env.BUILD === 'node') {
	config.kit.adapter = nodeAdapter({
		out: 'build-node'
	});
}

export default config;
