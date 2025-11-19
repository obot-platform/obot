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
	} catch (err) {
		handleRouteError(err, `/mcp-servers`, profile.current);
	}

	return {
		workspace
	};
};
