import { ChatService, type Project } from '$lib/services';
import { sortByFeaturedNameOrder } from '$lib/sort';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch }) => {
	const authProviders = await ChatService.listAuthProviders({ fetch });
	const featuredProjectShares = (await ChatService.listProjectShares({ fetch })).items
		.filter(
			// Ensure the project has a name and description before showing it on the unauthenticated
			// home page.
			(projectShare) => projectShare.name && projectShare.description && projectShare.icons?.icon
		)
		.sort(sortByFeaturedNameOrder);
	const tools = new Map((await ChatService.listAllTools({ fetch })).items.map((t) => [t.id, t]));
	let editorProjects: Project[] = [];

	try {
		editorProjects = (await ChatService.listProjects({ fetch })).items;
	} catch {
		// do nothing
	}

	if (editorProjects.length > 0) {
		const sortedProjects = editorProjects.sort(
			(a, b) => new Date(b.created).getTime() - new Date(a.created).getTime()
		);
		throw redirect(303, `/o/${sortedProjects[0].id}`);
	}

	return {
		authProviders,
		featuredProjectShares,
		tools
	};
};
