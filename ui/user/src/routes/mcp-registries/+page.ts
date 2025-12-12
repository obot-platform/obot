import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import type { AccessControlRule } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch }) => {
	let accessControlRules: AccessControlRule[] = [];
	let workspaceId;

	if (profile.current.hasAdminAccess?.()) {
		throw redirect(302, '/admin/mcp-registries');
	}

	try {
		workspaceId = await ChatService.fetchWorkspaceIDForProfile(profile.current?.id, { fetch });
		accessControlRules = await ChatService.listWorkspaceAccessControlRules(workspaceId, { fetch });
	} catch (err) {
		handleRouteError(err, '/mcp-registries', profile.current);
	}

	return {
		accessControlRules,
		workspaceId
	};
};
