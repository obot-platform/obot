import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { AccessControlRule } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, depends }) => {
	depends('mcp-registries:data');

	let accessControlRules: AccessControlRule[] = [];

	try {
		const adminAccessControlRules = await AdminService.listAccessControlRules({ fetch });
		const userWorkspacesAccessControlRules =
			await AdminService.listAllUserWorkspaceAccessControlRules({ fetch });
		accessControlRules = [...adminAccessControlRules, ...userWorkspacesAccessControlRules];
	} catch (err) {
		handleRouteError(err, '/admin/mcp-registries', profile.current);
	}

	return {
		accessControlRules
	};
};
