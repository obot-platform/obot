import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { wid, id } = params;

	let mcpServer;
	try {
		mcpServer = await ChatService.getWorkspaceMCPCatalogServer(wid, id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/mcp-servers/w/${wid}/s/${id}/details`, profile.current);
	}

	return {
		mcpServer,
		workspaceId: wid
	};
};
