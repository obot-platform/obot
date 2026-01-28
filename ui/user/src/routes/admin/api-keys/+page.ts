import { handleRouteError } from '$lib/errors';
import { AdminService, ApiKeysService } from '$lib/services';
import type { OrgUser } from '$lib/services/admin/types';
import type { APIKey } from '$lib/services/api-keys/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let myApiKeys: APIKey[] = [];
	let allApiKeys: APIKey[] = [];
	let users: OrgUser[] = [];

	try {
		const [keys, allKeys, userList] = await Promise.all([
			ApiKeysService.listApiKeys({ fetch }),
			ApiKeysService.listAllApiKeys({ fetch }),
			AdminService.listUsers({ fetch })
		]);

		myApiKeys = keys;
		allApiKeys = allKeys;
		users = userList;
	} catch (err) {
		handleRouteError(err, '/admin/api-keys', profile);
	}

	return { myApiKeys, allApiKeys, users };
};
