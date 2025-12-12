import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let workspace;

	if (profile?.hasAdminAccess?.()) {
		throw redirect(302, '/admin/mcp-servers');
	}

	try {
		const workspaces = await ChatService.listWorkspaces({ fetch });
		workspace = workspaces.find((w) => w.userID === profile?.id) ?? null;
	} catch (err) {
		handleRouteError(err, `/mcp-servers`, profile);
	}

	return {
		workspace
	};
};
