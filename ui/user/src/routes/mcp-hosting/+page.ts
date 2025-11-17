import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let workspace;
	try {
		const currentProfile = profile.current.id
			? profile.current
			: await ChatService.getProfile({ fetch });
		const workspaces = await ChatService.listWorkspaces({ fetch });
		workspace = workspaces.find((w) => w.userID === currentProfile.id) ?? null;

		if (!workspace) {
			throw new Error(
				'404 Workspace not found. If this problem persists, please contact an administrator.'
			);
		}
	} catch (err) {
		handleRouteError(err, `/admin/mcp-hosting`, profile.current);
	}

	return {
		workspace
	};
};
