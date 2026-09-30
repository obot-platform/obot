import { browser } from '$app/environment';
import { handleRouteError } from '$lib/errors';
import {
	AdminService,
	UserService,
	type MessagePolicy,
	type Model,
	type ModelProvider
} from '$lib/services';
import type { ModelAccessPolicy } from '$lib/services/admin/types';
import accessibleModels, { filterAccessibleModels } from '$lib/stores/accessibleModels.svelte';
import type { PageLoad } from './$types';

const views = new Set(['models', 'model-providers', 'access-policies', 'ai-judge-policies']);

export const load: PageLoad = async ({ fetch, parent, url }) => {
	const { profile, models: initialModels, version } = await parent();
	const hasAdminAccess = profile.hasAdminAccess?.() ?? false;
	const requestedView = url.searchParams.get('view');
	const view =
		requestedView &&
		views.has(requestedView) &&
		(hasAdminAccess || requestedView === 'models') &&
		(requestedView !== 'ai-judge-policies' || version?.messagePoliciesEnabled)
			? requestedView
			: 'models';

	let models: Model[] = [];
	let modelProviders: ModelProvider[] = [];
	let modelAccessPolicies: ModelAccessPolicy[] = [];
	let messagePolicies: MessagePolicy[] = [];

	if (view === 'models') {
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
			handleRouteError(err, '/models', profile);
		}
	} else if (hasAdminAccess) {
		switch (view) {
			case 'model-providers':
				try {
					modelProviders = await AdminService.listModelProviders({ fetch });
				} catch (err) {
					handleRouteError(err, '/models', profile);
				}
				break;
			case 'access-policies':
				try {
					modelAccessPolicies = await AdminService.listModelAccessPolicies({ fetch });
				} catch (err) {
					handleRouteError(err, '/models', profile);
				}
				break;
			case 'ai-judge-policies':
				try {
					messagePolicies = await AdminService.listMessagePolicies({ fetch });
				} catch (err) {
					handleRouteError(err, '/models', profile);
				}
				break;
		}
	}

	return {
		models,
		modelProviders,
		modelAccessPolicies,
		messagePolicies,
		hasAdminAccess
	};
};
