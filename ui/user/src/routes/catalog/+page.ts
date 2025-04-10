import { browser, building } from '$app/environment';
import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	// to be perfectly honest, I have no idea why this is needed, you'd assume the catch below would be enough
	if (building) {
		return {
			shares: [],
			tools: []
		};
	}

	try {
		const shares = ChatService.listProjectShares({ fetch });
		const tools = ChatService.listAllTools({ fetch });

		if (browser) {
			localStorage.setItem('hasVisitedCatalog', 'true');
		}

		return {
			shares: (await shares).items,
			tools: (await tools).items
		};
	} catch {
		return {
			shares: [],
			tools: []
		};
	}
};
