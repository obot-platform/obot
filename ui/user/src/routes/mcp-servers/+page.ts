import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch }) => {
	let workspace;

	if (profile.current.hasAdminAccess?.()) {
		throw redirect(302, '/admin/mcp-servers');
	}

	try {
		const currentProfile = profile.current.id
			? profile.current
			: await ChatService.getProfile({ fetch });
		const workspaces = await ChatService.listWorkspaces({ fetch });
		workspace = workspaces.find((w) => w.userID === currentProfile.id) ?? null;
	} catch (err) {
		handleRouteError(err, `/mcp-servers`, profile.current);
	}

	return {
		workspace
	};
};
