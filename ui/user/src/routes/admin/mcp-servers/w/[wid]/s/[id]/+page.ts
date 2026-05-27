import { handleRouteError, parseErrorContent } from '$lib/errors';
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
		} catch (err) {
			// Only swallow 404 — the referenced entry was deleted but the server still
			// points at it. Surface anything else so real failures aren't hidden.
			if (parseErrorContent(err).status !== 404) {
				handleRouteError(err, `/admin/mcp-servers/w/${wid}/s/${id}`, profile);
			}
		}
	}

	return {
		mcpServer,
		catalogEntry,
		workspaceId: wid,
		belongsToUser
	};
};
