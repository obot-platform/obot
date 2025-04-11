import { browser, building } from '$app/environment';
import { ChatService, type Project } from '$lib/services';
import { sortByFeaturedNameOrder } from '$lib/sort';
import { qIsSet } from '$lib/url';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch }) => {
	if (building) {
		return {
			authProviders: [],
			featuredProjectShares: [],
			tools: new Map()
		};
	}
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

	if (browser) {
		const redirectSet = qIsSet('rd');
		const lastVisitedObot = localStorage.getItem('lastVisitedObot');
		const matchingProject = editorProjects.find((p) => p.id === lastVisitedObot);
		if (lastVisitedObot && matchingProject && !redirectSet) {
			throw redirect(303, `/o/${matchingProject.id}`);
		}
	}

	return {
		authProviders,
		featuredProjectShares,
		tools
	};
};
