import { type Assistant, ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const authProviders = await ChatService.listAuthProviders({ fetch });
	const featuredProjectShares = (await ChatService.listProjectShares({ fetch })).items.filter(
		// Ensure the project has a name and description before showing it on the unauthenticated
		// home page.
		(projectShare) => projectShare.name && projectShare.description && projectShare.icons?.icon
	);
	const tools = new Map((await ChatService.listAllTools({ fetch })).items.map((t) => [t.id, t]));
	let assistantsLoaded = false;
	let assistants: Assistant[] = [];

	try {
		assistants = (await ChatService.listAssistants({ fetch })).items;
		assistantsLoaded = true;
	} catch {
		// do nothing
	}

	return {
		authProviders,
		assistants,
		assistantsLoaded,
		featuredProjectShares,
		tools
	};
};
