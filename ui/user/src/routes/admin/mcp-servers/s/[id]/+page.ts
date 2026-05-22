import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;

	let mcpServer;
	try {
		mcpServer = await UserService.getMcpCatalogServer(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/mcp-servers/s/${id}`, profile.current);
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
		catalogEntry
	};
};
