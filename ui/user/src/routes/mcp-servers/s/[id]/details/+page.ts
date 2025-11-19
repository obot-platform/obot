import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;

	let workspaceId;
	let mcpServer;
	try {
		workspaceId = await ChatService.fetchWorkspaceIDForProfile(profile.current?.id, { fetch });
		mcpServer = await ChatService.getWorkspaceMCPCatalogServer(workspaceId, id, {
			fetch
		});
	} catch (err) {
		handleRouteError(err, `/mcp-servers/s/${id}/details`, profile.current);
	}

	return {
		workspaceId,
		mcpServer,
		id
	};
};
