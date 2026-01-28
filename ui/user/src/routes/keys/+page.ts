import { handleRouteError } from '$lib/errors';
import { ApiKeysService } from '$lib/services';
import type { APIKey } from '$lib/services/api-keys/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let apiKeys: APIKey[] = [];

	try {
		const [keys] = await Promise.all([ApiKeysService.listApiKeys({ fetch })]);

		apiKeys = keys;
	} catch (err) {
		handleRouteError(err, '/keys', profile);
	}

	return { apiKeys };
};
