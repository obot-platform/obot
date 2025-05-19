import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const projects = (await ChatService.listProjects({ fetch })).items;
		const templates = (await ChatService.listTemplates({ fetch })).items;
		const shares = (await ChatService.listProjectShares({ fetch })).items;
		const tools = (await ChatService.listAllTools({ fetch })).items;

		return {
			projects,
			templates,
			shares,
			tools
		};
	} catch {
		return {
			projects: [],
			templates: [],
			shares: [],
			tools: []
		};
	}
};
