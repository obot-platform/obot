import { type Assistant, ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const authProviders = await ChatService.listAuthProviders({ fetch });
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
		assistantsLoaded
	};
};
