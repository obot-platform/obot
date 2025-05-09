import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const templates = (await ChatService.listTemplates({ fetch })).items;
		const mcps = await ChatService.listMCPs({ fetch });
		const tools = (await ChatService.listAllTools({ fetch })).items;

		return {
			templates,
			mcps,
			tools
		};
	} catch {
		return {
			templates: [],
			mcps: [],
			tools: []
		};
	}
};
