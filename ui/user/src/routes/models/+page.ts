import { handleRouteError } from '$lib/errors';
import { ChatService, type Model } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let models: Model[] = [];
	try {
		models = await ChatService.listModels({ fetch });
	} catch (err) {
		handleRouteError(err, '/models', profile.current);
	}

	return {
		models
	};
};
