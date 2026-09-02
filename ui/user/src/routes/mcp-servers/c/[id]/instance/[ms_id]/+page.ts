import { handleRouteError } from '$lib/errors';
import { getMCPCatalogEntry, getSingleOrRemoteMcpServer } from '../../../../utils';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, url, fetch, parent }) => {
	const catalogEntryId = params.id;
	const mcpServerId = params.ms_id;
	const { profile } = await parent();

	let catalogEntry;
	let mcpServer;
	try {
		catalogEntry = await getMCPCatalogEntry(catalogEntryId, url, profile, fetch);
		mcpServer = await getSingleOrRemoteMcpServer(mcpServerId, catalogEntryId, url, profile, fetch);
	} catch (err) {
		handleRouteError(err, `/mcp-servers/c/${catalogEntryId}/instance/${mcpServerId}`, profile);
	}

	return {
		catalogEntry,
		mcpServerId,
		mcpServer
	};
};
