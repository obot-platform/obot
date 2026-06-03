import { CommonModelProviderIds } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import { UserService, type Model } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

const SUPPORTED_PROVIDER_IDS = new Set<string>([
	CommonModelProviderIds.OPENAI,
	CommonModelProviderIds.ANTHROPIC
]);

export const load: PageLoad = async ({ fetch }) => {
	let models: Model[] = [];

	try {
		const all = await UserService.listModels({ fetch });
		models = all.filter((m) => m.active && SUPPORTED_PROVIDER_IDS.has(m.modelProvider));
	} catch (err) {
		handleRouteError(err, '/llm-gateway/models', profile.current);
	}

	return { models };
};
