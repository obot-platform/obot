import { handleRouteError, parseErrorContent } from '$lib/errors';
import { UserService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;

	let mcpServer;
	let workspaceId;
	try {
		workspaceId = await UserService.fetchWorkspaceIDForProfile(profile.current?.id, { fetch });
	} catch (_err) {
		// may not have a workspace id if basic user atm
		workspaceId = undefined;
	}

	try {
		mcpServer = await UserService.getMcpCatalogServer(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/mcp-servers/s/${id}`, profile.current);
	}

	let catalogEntry;
	const isOwner =
		profile.current?.isAdmin?.() ||
		(mcpServer?.powerUserWorkspaceID && mcpServer?.userID === profile.current?.id);
	if (mcpServer?.catalogEntryID && isOwner) {
		try {
			catalogEntry = await UserService.getMCP(mcpServer.catalogEntryID, { fetch });
		} catch (err) {
			// Only swallow 404 — the referenced entry was deleted but the server still
			// points at it. Surface anything else so real failures aren't hidden.
			if (parseErrorContent(err).status !== 404) {
				handleRouteError(err, `/mcp-servers/s/${id}`, profile.current);
			}
		}
	}

	return {
		mcpServer,
		catalogEntry,
		workspaceId
	};
};
