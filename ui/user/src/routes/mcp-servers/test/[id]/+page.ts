import { handleRouteError } from '$lib/errors';
import { UserService, type MCPCatalogServer } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

function safeBackTarget(server: MCPCatalogServer): string {
	if (server.catalogEntryID && server.serverUserType === 'singleUser') {
		return `/mcp-servers/c/${encodeURIComponent(server.catalogEntryID)}/instance/${encodeURIComponent(server.id)}`;
	}
	return `/mcp-servers/s/${encodeURIComponent(server.id)}`;
}

export const load: PageLoad = async ({ params, fetch }) => {
	const path = `/mcp-servers/test/${params.id}`;
	try {
		const server = await UserService.getMCPTesterServer(params.id, { fetch });
		return {
			server,
			backTarget: safeBackTarget(server)
		};
	} catch (error) {
		handleRouteError(error, path, profile.current);
	}
};
