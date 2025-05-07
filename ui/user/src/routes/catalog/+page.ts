import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const templates = (await ChatService.listTemplates({ fetch })).items;
		const mcps = await ChatService.listMCPs({ fetch });

		return {
			templates,
			mcps
		};
	} catch {
		return {
			templates: [],
			mcps: []
		};
	}
};
