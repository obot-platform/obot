import { handleRouteError } from '$lib/errors';
import { AdminService, UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { id } = params;
	const { profile } = await parent();

	let accessControlRule;
	let workspaceId: string | undefined;

	try {
		if (profile.hasAdminAccess?.()) {
			accessControlRule = await AdminService.getAccessControlRule(id, { fetch });
		} else {
			workspaceId = await UserService.fetchWorkspaceIDForProfile(profile?.id, { fetch });
			accessControlRule = await UserService.getWorkspaceAccessControlRule(workspaceId, id, {
				fetch
			});
		}
	} catch (err) {
		handleRouteError(err, `/mcp-servers/access-policies/${id}`, profile);
	}

	return {
		accessControlRule,
		workspaceId
	};
};
