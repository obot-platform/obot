import { handleRouteError } from '$lib/errors';
import { ApiKeysService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, parent, fetch }) => {
	const { profile } = await parent();
	const { id } = params;
	const isAdmin = profile.hasAdminAccess?.() ?? false;
	let apiKey;
	try {
		apiKey = await (isAdmin ? ApiKeysService.getAnyApiKey : ApiKeysService.getApiKey)(id, {
			fetch
		});
	} catch (err) {
		handleRouteError(err, `/identity-access/agents/${id}`, profile);
	}
	return {
		apiKey,
		isAdmin
	};
};
