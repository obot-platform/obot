import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const editorProjects = ChatService.listProjects({ fetch });
		const shares = ChatService.listProjectShares({ fetch });
		const tools = ChatService.listAllTools({ fetch });
		return {
			editorProjects: (await editorProjects).items,
			shares: (await shares).items,
			tools: (await tools).items
		};
	} catch {
		return {
			editorProjects: [],
			shares: [],
			tools: []
		};
	}
};
