import { browser } from '$app/environment';
import { handleRouteError } from '$lib/errors';
import { AdminService, UserService, type Model, type ModelProvider } from '$lib/services';
import type { ModelAccessPolicy } from '$lib/services/admin/types';
import accessibleModels, { filterAccessibleModels } from '$lib/stores/accessibleModels.svelte';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile, models: initialModels } = await parent();
	const hasAdminAccess = profile.hasAdminAccess?.() ?? false;

	let models: Model[] = [];
	try {
		const response =
			browser && accessibleModels.initialized && accessibleModels.current.length > 0
				? accessibleModels.current
				: (initialModels ?? (await UserService.listModels({ fetch })));

		models = filterAccessibleModels(response ?? []);

		if (browser) {
			accessibleModels.set(models);
		}
	} catch (err) {
		handleRouteError(err, '/v2/models', profile);
	}

	if (!hasAdminAccess && models.length === 0) {
		throw redirect(302, '/');
	}

	let modelProviders: ModelProvider[] = [];
	let modelAccessPolicies: ModelAccessPolicy[] = [];

	if (hasAdminAccess) {
		try {
			modelProviders = await AdminService.listModelProviders({ fetch });
		} catch (err) {
			handleRouteError(err, '/v2/models', profile);
		}

		try {
			modelAccessPolicies = await AdminService.listModelAccessPolicies({ fetch });
		} catch (err) {
			handleRouteError(err, '/v2/models', profile);
		}
	}

	return {
		models,
		modelProviders,
		modelAccessPolicies,
		hasAdminAccess
	};
};
