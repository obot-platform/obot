import { browser } from '$app/environment';
import { ChatService, type Project, type ToolReference } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch }) => {
	let editorProjects: Project[] = [];
	let tools: ToolReference[] = [];

	try {
		editorProjects = (await ChatService.listProjects({ fetch })).items;
		tools = (await ChatService.listAllTools({ fetch })).items;
	} catch {
		return { editorProjects, tools };
	}

	if (editorProjects.length === 0 && browser && !localStorage.getItem('hasVisitedCatalog')) {
		throw redirect(303, '/catalog?new');
	}

	return {
		editorProjects,
		tools
	};
};
