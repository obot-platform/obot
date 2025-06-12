import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const mcps = await ChatService.listMCPs({ fetch });
		const tools = (await ChatService.listAllTools({ fetch })).items;

		return {
			mcps,
			tools
		};
	} catch {
		return {
			mcps: [],
			tools: []
		};
	}
};
