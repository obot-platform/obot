import { handleRouteError } from '$lib/errors';
import { AdminService, type License, type ModelProvider } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let modelProviders: ModelProvider[] = [];
	let license: License | undefined = undefined;
	try {
		modelProviders = await AdminService.listModelProviders({ fetch });
		license = await AdminService.getLicense({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/model-providers', profile.current);
	}

	return {
		license,
		modelProviders
	};
};
