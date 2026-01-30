import { handleRouteError } from '$lib/errors';
import { ApiKeysService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, parent, fetch }) => {
	const { profile } = await parent();
	const { id } = params;
	let apiKey;
	try {
		apiKey = await ApiKeysService.getAnyApiKey(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/api-keys/${id}`, profile);
	}
	return {
		apiKey
	};
};
