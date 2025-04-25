import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const mcps = await ChatService.listMCPs({ fetch });
		return { mcps };
	} catch {
		return {
			mcps: []
		};
	}
};
