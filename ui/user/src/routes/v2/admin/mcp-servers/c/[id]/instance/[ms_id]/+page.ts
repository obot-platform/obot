import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import { AdminService, ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { error } from '@sveltejs/kit';

export const load: PageLoad = async ({ params, fetch }) => {
	const catalogEntryId = params.id;
	const serverId = params.ms_id;

	let catalogServer;
	let catalogEntry;
	try {
		catalogEntry = await AdminService.getMCPCatalogEntry(DEFAULT_MCP_CATALOG_ID, catalogEntryId, {
			fetch
		});
		catalogServer = await ChatService.getSingleOrRemoteMcpServer(serverId, {
			fetch
		});
		if (!catalogServer || catalogServer.catalogEntryID !== catalogEntryId) {
			throw error(404, 'MCP server for catalog not found');
		}
	} catch (err) {
		handleRouteError(
			err,
			`/v2/admin/mcp-servers/c/${catalogEntryId}/instance/${serverId}`,
			profile.current
		);
	}

	return {
		catalogEntry,
		catalogServer
	};
};
