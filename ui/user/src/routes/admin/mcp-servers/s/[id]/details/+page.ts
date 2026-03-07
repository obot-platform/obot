import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import { AdminService, ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;
	const isNanobotServer = id.startsWith('ms1nba1');

	let mcpServer;
	if (!isNanobotServer) {
		try {
			mcpServer = await AdminService.getMCPCatalogServer(DEFAULT_MCP_CATALOG_ID, id, {
				fetch
			});
		} catch (_err) {
			try {
				mcpServer = await ChatService.getSingleOrRemoteMcpServer(id, { fetch });
			} catch (err) {
				handleRouteError(err, `/admin/mcp-servers/s/${id}/details`, profile.current);
			}
		}
	} else {
		try {
			mcpServer = await ChatService.getSingleOrRemoteMcpServer(id, { fetch });
		} catch (err) {
			handleRouteError(err, `/admin/mcp-servers/s/${id}/details`, profile.current);
		}
	}

	return {
		mcpServer,
		id
	};
};
