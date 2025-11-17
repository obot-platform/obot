import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;

	let mcpServer;
	let workspaceId;
	try {
		workspaceId = await ChatService.fetchWorkspaceIDForProfile(profile.current?.id, { fetch });
		mcpServer = await ChatService.getWorkspaceMCPCatalogServer(workspaceId, id, {
			fetch
		});
	} catch (err) {
		handleRouteError(err, `/mcp-hosting/s/${id}`, profile.current);
	}

	return {
		mcpServer,
		workspaceId
	};
};
