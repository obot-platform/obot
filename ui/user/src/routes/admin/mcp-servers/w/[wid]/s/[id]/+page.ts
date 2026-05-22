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

	let catalogEntry;
	if (mcpServer?.catalogEntryID) {
		try {
			catalogEntry = await UserService.getMCP(mcpServer.catalogEntryID, { fetch });
		} catch (_err) {
			// Entry may not be accessible
		}
	}

	return {
		mcpServer,
		catalogEntry,
		workspaceId: wid,
		belongsToUser
	};
};
