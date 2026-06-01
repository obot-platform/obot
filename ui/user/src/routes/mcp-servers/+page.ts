import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let workspace;

	try {
		const workspaces = await UserService.listWorkspaces({ fetch });
		workspace = workspaces.find((w) => w.userID === profile?.id) ?? null;
	} catch (err) {
		handleRouteError(err, `/mcp-servers`, profile);
	}

	return {
		workspace
	};
};
