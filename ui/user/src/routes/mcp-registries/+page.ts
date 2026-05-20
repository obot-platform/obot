import { handleRouteError } from '$lib/errors';
import { UserService, type AccessControlRule } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	let accessControlRules: AccessControlRule[] = [];
	let workspaceId;

	const { profile } = await parent();
	if (profile?.hasAdminAccess?.()) {
		throw redirect(302, '/admin/mcp-registries');
	}

	try {
		workspaceId = await UserService.fetchWorkspaceIDForProfile(profile?.id, { fetch });
		accessControlRules = await UserService.listWorkspaceAccessControlRules(workspaceId, { fetch });
	} catch (err) {
		handleRouteError(err, '/mcp-registries', profile);
	}

	return {
		accessControlRules,
		workspaceId
	};
};
