import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { id, wid } = params;
	const { profile } = await parent();

	let belongsToUser;
	let mcpServer;
	try {
		mcpServer = await UserService.getWorkspaceMCPCatalogServer(wid, id, {
			fetch
		});
	} catch (err) {
		handleRouteError(err, `/admin/mcp-servers/w/${wid}/s/${id}`, profile);
	}

	try {
		const userWorkspaceId = await UserService.fetchWorkspaceIDForProfile(profile.id, { fetch });
		belongsToUser = userWorkspaceId === wid;
	} catch (_err) {
		belongsToUser = false;
	}

	return {
		mcpServer,
		workspaceId: wid,
		belongsToUser
	};
};
