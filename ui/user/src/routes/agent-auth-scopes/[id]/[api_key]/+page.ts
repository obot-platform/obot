import { handleRouteError } from '$lib/errors';
import { ApiKeysService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

const ADMIN_AGENT_AUTH_SCOPES_PREFIX = '/admin/agent-auth-scopes';

export const load: PageLoad = async ({ params, parent, fetch, url }) => {
	const { profile } = await parent();
	const { id, api_key } = params;
	const isAdmin =
		url.pathname === ADMIN_AGENT_AUTH_SCOPES_PREFIX ||
		url.pathname.startsWith(`${ADMIN_AGENT_AUTH_SCOPES_PREFIX}/`);

	if (!profile.hasAdminAccess?.()) {
		throw redirect(302, '/');
	}

	let apiKey;
	try {
		apiKey = await (isAdmin ? ApiKeysService.getAnyApiKey : ApiKeysService.getApiKey)(id, {
			fetch
		});
	} catch (err) {
		handleRouteError(err, `${isAdmin ? '/admin' : ''}/agent-auth-scopes/${id}/${api_key}`, profile);
	}
	return {
		apiKey,
		apiKeyId: api_key,
		isAdmin
	};
};
