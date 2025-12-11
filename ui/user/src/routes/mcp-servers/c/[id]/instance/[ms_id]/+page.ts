import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const catalogEntryId = params.id;
	const mcpServerId = params.ms_id;
	let workspaceId;
	let catalogEntry;
	let mcpServer;
	try {
		workspaceId = await ChatService.fetchWorkspaceIDForProfile(profile.current?.id, { fetch });
	} catch (_err) {
		// can happen if basic user atm
		workspaceId = undefined;
	}

	try {
		catalogEntry = await ChatService.getMCP(catalogEntryId, {
			fetch
		});
		mcpServer = await ChatService.getSingleOrRemoteMcpServer(mcpServerId, { fetch });
	} catch (err) {
		handleRouteError(
			err,
			`/mcp-servers/c/${catalogEntryId}/instance/${mcpServerId}`,
			profile.current
		);
	}

	return {
		workspaceId,
		catalogEntry,
		mcpServer,
		mcpServerId
	};
};
