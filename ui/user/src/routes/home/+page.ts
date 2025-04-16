import { ChatService, type Project, type ToolReference } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let editorProjects: Project[] = [];
	let tools: ToolReference[] = [];

	try {
		editorProjects = (await ChatService.listProjects({ fetch })).items;
		tools = (await ChatService.listAllTools({ fetch })).items;
	} catch {
		return { editorProjects, tools };
	}

	return {
		editorProjects,
		tools
	};
};
