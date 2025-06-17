import { handleRouteError } from '$lib/errors';
import { AdminService, type ModelProvider } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let modelProviders: ModelProvider[] = [];
	try {
		modelProviders = await AdminService.listModelProviders({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/v2/model-providers', profile.current);
	}

	return {
		modelProviders
	};
};
