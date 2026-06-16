import { browser } from '$app/environment';
import { handleRouteError } from '$lib/errors';
import { UserService, type Model } from '$lib/services';
import accessibleModels, { filterAccessibleModels } from '$lib/stores/accessibleModels.svelte';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let models: Model[] = [];

	try {
		const all = await UserService.listModels({ fetch });
		models = filterAccessibleModels(all);
		if (browser) {
			accessibleModels.set(all);
		}
	} catch (err) {
		handleRouteError(err, '/llm-gateway/models', profile);
	}

	if (models.length === 0) {
		throw redirect(302, '/');
	}

	return { models };
};
