import { handleRouteError } from '$lib/errors';
import { ApiKeysService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ params, parent, fetch }) => {
	const { profile } = await parent();
	const { id, api_key } = params;
	const isAdmin = profile.hasAdminAccess?.() ?? false;

	if (!profile.hasAdminAccess?.()) {
		throw redirect(302, '/');
	}

	let apiKey;
	try {
		apiKey = await (isAdmin ? ApiKeysService.getAnyApiKey : ApiKeysService.getApiKey)(id, {
			fetch
		});
	} catch (err) {
		handleRouteError(err, `/identity-access/agents/${id}/${api_key}`, profile);
	}

	return {
		apiKey,
		apiKeyId: api_key,
		isAdmin
	};
};
