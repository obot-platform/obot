import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const catalogEntryId = params.id;
	const mcpServerId = params.ms_id;

	let catalogEntry;
	try {
		catalogEntry = await AdminService.getMCPCatalogEntry(DEFAULT_MCP_CATALOG_ID, catalogEntryId, {
			fetch
		});
	} catch (err) {
		handleRouteError(
			err,
			`/v2/admin/mcp-servers/c/${catalogEntryId}/instance/${mcpServerId}`,
			profile.current
		);
	}

	return {
		catalogEntry,
		mcpServerId
	};
};
