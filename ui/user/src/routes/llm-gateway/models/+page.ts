import { browser } from '$app/environment';
import { handleRouteError } from '$lib/errors';
import { UserService, type Model } from '$lib/services';
import accessibleModels, { filterAccessibleModels } from '$lib/stores/accessibleModels.svelte';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

// Distinguishes the first client run (SSR hydration) from later client-side navigations.
let hasHydrated = false;

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile, models: initialModels } = await parent();
	let models: Model[] = [];

	try {
		let response: Model[] | undefined;

		if (import.meta.env.SSR && initialModels) {
			response = initialModels;
		} else if (!hasHydrated && initialModels) {
			hasHydrated = true;
			response = initialModels;
		} else {
			hasHydrated = true;
			response = await UserService.listModels({ fetch });
		}

		models = filterAccessibleModels(response ?? []);

		if (browser) {
			accessibleModels.set(models);
		}
	} catch (err) {
		handleRouteError(err, '/llm-gateway/models', profile);
	}

	if (models.length === 0) {
		throw redirect(302, '/');
	}

	return { models };
};
