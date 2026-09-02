import { handleRouteError, parseErrorContent } from '$lib/errors';
import { getMCPCatalogEntry, getMCPCatalogServer } from '../../utils';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, url, fetch, parent }) => {
	const { profile } = await parent();
	const { id } = params;

	let mcpServer;
	try {
		mcpServer = await getMCPCatalogServer(id, url, profile, fetch);
	} catch (err) {
		handleRouteError(err, `/mcp-servers/s/${id}`, profile);
	}

	let catalogEntry;
	if (mcpServer?.catalogEntryID) {
		try {
			catalogEntry = await getMCPCatalogEntry(mcpServer.catalogEntryID, url, profile, fetch);
		} catch (err) {
			// Only swallow 404 — the referenced entry was deleted but the server still
			// points at it. Surface anything else so real failures aren't hidden.
			if (parseErrorContent(err).status !== 404) {
				handleRouteError(err, `/mcp-servers/s/${id}`, profile);
			}
		}
	}

	return {
		mcpServer,
		catalogEntry
	};
};
