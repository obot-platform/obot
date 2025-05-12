import { browser, building } from '$app/environment';
import { ChatService, type Project } from '$lib/services';
import { qIsSet } from '$lib/url';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch }) => {
	if (building) {
		return {
			authProviders: [],
			tools: [],
			mcps: [],
			templates: []
		};
	}

	const authProviders = await ChatService.listAuthProviders({ fetch });
	const templates = (await ChatService.listTemplates({ fetch })).items.filter(
		(template) => template.featured
	);
	const mcps = await ChatService.listMCPs({ fetch });
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
		isNew: editorProjects.length === 0,
		authProviders,
		mcps,
		templates
	};
};
