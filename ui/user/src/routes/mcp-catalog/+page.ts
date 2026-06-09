import { Group, UserService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();

	const isPowerUserOrAdmin = profile.groups.includes(Group.POWERUSER) || profile.hasAdminAccess?.();

	if (!isPowerUserOrAdmin) {
		throw redirect(302, '/mcp-servers');
	}

	try {
		const workspaceId = await UserService.fetchWorkspaceIDForProfile(profile.id, { fetch });
		return {
			workspaceId
		};
	} catch (_err) {
		// ex. may not have a workspaceId if basic user with auditor access
		return {
			workspaceId: undefined
		};
	}
};
