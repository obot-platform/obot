import { init } from '$lib/stores/context.svelte';

export const prerender = 'auto';

export async function load({ params }) {
	const { agent, project } = params;
	await init(agent, {
		projectID: project
	});
}
