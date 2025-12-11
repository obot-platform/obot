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
	} catch (_err) {
		// may not have a workspace id if basic user atm
		workspaceId = undefined;
	}

	try {
		mcpServer = await ChatService.getMcpCatalogServer(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/mcp-servers/s/${id}`, profile.current);
	}

	return {
		mcpServer,
		workspaceId
	};
};
