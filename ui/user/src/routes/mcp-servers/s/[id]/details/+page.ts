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
	} catch (_err) {
		// can happen if basic user atm
		workspaceId = undefined;
	}

	try {
		mcpServer = await ChatService.getMcpCatalogServer(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/mcp-servers/s/${id}/details`, profile.current);
	}
	return {
		workspaceId,
		mcpServer,
		id
	};
};
