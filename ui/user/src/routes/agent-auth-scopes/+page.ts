import { handleRouteError } from '$lib/errors';
import { ApiKeysService, UserService, type OrgUser } from '$lib/services';
import type { APIKey } from '$lib/services/api-keys/types';
import type { PageLoad } from './$types';

const ADMIN_AGENT_AUTH_SCOPES_PREFIX = '/admin/agent-auth-scopes';

export const load: PageLoad = async ({ fetch, parent, url }) => {
	const { profile } = await parent();
	const isAdmin =
		url.pathname === ADMIN_AGENT_AUTH_SCOPES_PREFIX ||
		url.pathname.startsWith(`${ADMIN_AGENT_AUTH_SCOPES_PREFIX}/`);
	let apiKeys: APIKey[] = [];
	let users: OrgUser[] = [];

	try {
		if (isAdmin) {
			[apiKeys, users] = await Promise.all([
				ApiKeysService.listAllApiKeys({ fetch }),
				UserService.listUsers({ fetch })
			]);
		} else {
			apiKeys = await ApiKeysService.listApiKeys({ fetch });
		}
	} catch (err) {
		handleRouteError(err, `${isAdmin ? '/admin' : ''}/agent-auth-scopes`, profile);
	}

	return { apiKeys, users, isAdmin };
};
