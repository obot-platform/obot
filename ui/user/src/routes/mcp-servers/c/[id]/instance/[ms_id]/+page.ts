import { handleRouteError } from '$lib/errors';
import { getMCPCatalogEntry, getSingleOrRemoteMcpServer } from '../../../../utils';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, url, fetch, parent }) => {
	const catalogEntryId = params.id;
	const mcpServerId = params.ms_id;
	const { profile } = await parent();
	const wid = url.searchParams.get('wid');

	let catalogEntry;
	let mcpServer;
	try {
		catalogEntry = await getMCPCatalogEntry(catalogEntryId, wid, profile, fetch);
		mcpServer = await getSingleOrRemoteMcpServer(mcpServerId, catalogEntryId, wid, profile, fetch);
	} catch (err) {
		handleRouteError(
			err,
			`/mcp-servers/c/${catalogEntryId}/instance/${mcpServerId}`,
			profile
		);
	}

	return {
		catalogEntry,
		mcpServerId,
		mcpServer
	};
};
