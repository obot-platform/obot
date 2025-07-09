import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { error } from '@sveltejs/kit';

export const load: PageLoad = async ({ params, fetch }) => {
	const serverId = params.id;
	const instanceId = params.msi_id;

	let mcpServer;
	let mcpServerInstance;
	try {
		mcpServer = await AdminService.getMCPCatalogServer(DEFAULT_MCP_CATALOG_ID, serverId, {
			fetch
		});
		const instances = await AdminService.listMcpCatalogServerInstances(
			DEFAULT_MCP_CATALOG_ID,
			serverId,
			{
				fetch
			}
		);
		mcpServerInstance = instances.find((i) => i.id === instanceId);

		if (!mcpServerInstance) {
			throw error(404, 'MCP server instance not found');
		}
	} catch (err) {
		handleRouteError(
			err,
			`/v2/admin/mcp-servers/s/${serverId}/instance/${instanceId}`,
			profile.current
		);
	}

	return {
		mcpServer,
		mcpServerInstance
	};
};
