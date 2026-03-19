import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;

	let mcpServer;
	try {
		mcpServer = await AdminService.getMCPCatalogServer(DEFAULT_MCP_CATALOG_ID, id, {
			fetch
		});
	} catch (err) {
		handleRouteError(err, `/admin/mcp-servers/s/${id}/details`, profile.current);
	}

	return {
		mcpServer,
		id
	};
};
