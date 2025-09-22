import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id, wid } = params;

	let mcpServer;
	try {
		mcpServer = await ChatService.getSingleOrRemoteMcpServer(id, {
			fetch
		});
	} catch (err) {
		handleRouteError(err, `/admin/mcp-servers/w/${wid}/s/${id}`, profile.current);
	}

	return {
		mcpServer,
		workspaceId: wid
	};
};
