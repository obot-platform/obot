import { browser } from '$app/environment';
import { handleRouteError } from '$lib/errors';
import { UserService, type Model } from '$lib/services';
import accessibleModels, { filterAccessibleModels } from '$lib/stores/accessibleModels.svelte';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

let isInitialLoad = true;

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile, models: parentModels } = await parent();
	let models: Model[] = [];

	const reuseParentModels = isInitialLoad;
	isInitialLoad = false;

	try {
		const all =
			reuseParentModels && parentModels ? parentModels : await UserService.listModels({ fetch });
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
