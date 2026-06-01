import { handleRouteError } from '$lib/errors';
import { AdminService, type AccessControlRule } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, depends }) => {
	depends('mcp-access-policies:data');

	let accessControlRules: AccessControlRule[] = [];

	try {
		const adminAccessControlRules = await AdminService.listAccessControlRules({ fetch });
		const userWorkspacesAccessControlRules =
			await AdminService.listAllUserWorkspaceAccessControlRules({ fetch });
		accessControlRules = [...adminAccessControlRules, ...userWorkspacesAccessControlRules];
	} catch (err) {
		handleRouteError(err, '/admin/mcp-access-policies', profile.current);
	}

	return {
		accessControlRules
	};
};
