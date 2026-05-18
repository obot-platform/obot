import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const catalogEntryId = params.id;
	const mcpServerId = params.ms_id;
	let workspaceId;
	let catalogEntry;
	let mcpServer;
	try {
		workspaceId = await UserService.fetchWorkspaceIDForProfile(profile.current?.id, { fetch });
	} catch (_err) {
		// can happen if basic user atm
		workspaceId = undefined;
	}

	try {
		catalogEntry = await UserService.getMCP(catalogEntryId, {
			fetch
		});
		mcpServer = await UserService.getSingleOrRemoteMcpServer(mcpServerId, { fetch });
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
