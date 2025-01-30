import { context } from '$lib/stores';

export const prerender = 'auto';

export async function load({ params }) {
	const { agent } = params;

	context.setContext({
		assistantID: agent,
		projectID: 'default',
		valid: true
	});
}
