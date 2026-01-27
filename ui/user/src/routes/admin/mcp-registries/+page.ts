import { handleRouteError } from '$lib/errors';
import { AdminService, ChatService } from '$lib/services';
import type { AccessControlRule } from '$lib/services/admin/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, depends, parent }) => {
	depends('mcp-registries:data');
	const { profile } = await parent();

	let accessControlRules: AccessControlRule[] = [];
	let workspaceId;

	try {
		const adminAccessControlRules = await AdminService.listAccessControlRules({ fetch });
		const userWorkspacesAccessControlRules =
			await AdminService.listAllUserWorkspaceAccessControlRules({ fetch });
		accessControlRules = [...adminAccessControlRules, ...userWorkspacesAccessControlRules];
	} catch (err) {
		handleRouteError(err, '/admin/mcp-registries', profile);
	}
	try {
		workspaceId = await ChatService.fetchWorkspaceIDForProfile(profile.id, { fetch });
	} catch (_err) {
		// ex. may not have a workspaceId if basic user with auditor access
		workspaceId = undefined;
	}
	return {
		accessControlRules,
		workspaceId
	};
};
