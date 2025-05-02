import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const shares = (await ChatService.listProjectShares({ fetch })).items;
		const tools = (await ChatService.listAllTools({ fetch })).items;
		const mcps = await ChatService.listMCPs({ fetch });

		return {
			shares,
			tools,
			mcps
		};
	} catch {
		return {
			shares: [],
			tools: [],
			mcps: []
		};
	}
};
