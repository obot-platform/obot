import { browser } from '$app/env';
import { handleRouteError } from '$lib/errors';
import { ApiKeysService, UserService, type APIKey, type Model } from '$lib/services';
import accessibleModels, { filterAccessibleModels } from '$lib/stores/accessibleModels.svelte';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

const views = new Set(['connect', 'providers', 'profiles', 'guardrails', 'routing', 'settings']);

// Links from earlier releases point at tabs that moved: the model providers tab is now the
// Providers tab, and access policies and AI judge policies became parts of profiles.
const viewAliases: Record<string, string> = {
	models: 'connect',
	'model-providers': 'providers',
	'access-policies': 'profiles',
	'ai-judge-policies': 'profiles'
};

export const load: PageLoad = async ({ fetch, parent, url }) => {
	const alias = viewAliases[url.searchParams.get('view') ?? ''];
	if (alias) {
		const searchParams = new URLSearchParams(url.searchParams);
		searchParams.set('view', alias);
		searchParams.delete('new');
		throw redirect(307, `${url.pathname}?${searchParams}`);
	}

	const { profile, models: initialModels } = await parent();
	const hasAdminAccess = profile.hasAdminAccess?.() ?? false;
	const requestedView = url.searchParams.get('view');
	const view =
		requestedView && views.has(requestedView) && (hasAdminAccess || requestedView === 'connect')
			? requestedView
			: 'connect';

	let models: Model[] = [];
	let apiKeys: APIKey[] = [];

	if (view === 'connect') {
		const keysPromise = ApiKeysService.listApiKeys({ fetch }).catch((): APIKey[] => []);
		try {
			const response = await (browser &&
			accessibleModels.initialized &&
			accessibleModels.current.length > 0
				? accessibleModels.current
				: (initialModels ?? UserService.listModels({ fetch })));

			models = filterAccessibleModels(response ?? []);

			if (browser) {
				accessibleModels.set(models);
			}
		} catch (err) {
			handleRouteError(err, '/models', profile);
		}
		apiKeys = await keysPromise;
	}

	return {
		models,
		apiKeys,
		hasAdminAccess
	};
};
