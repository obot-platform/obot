import { EditorService, type Project } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ url, fetch }) => {
	const mcpId = url.searchParams.get('id');
	let project: Project;
	try {
		project = await EditorService.createObot({ fetch });
		// Redirect to the new Obot with the mcp parameter if provided
		const redirectUrl = mcpId ? `/o/${project.id}?mcp=${mcpId}` : `/o/${project.id}`;
		throw redirect(303, redirectUrl);
	} catch (err) {
		if (!(err instanceof Error)) {
			throw err;
		}

		// unauthorized, go home
		if (err.message?.includes('401') || err.message?.includes('unauthorized')) {
			throw redirect(303, `/`);
		}

		// otherwise, let error throw
		throw err;
	}
};
