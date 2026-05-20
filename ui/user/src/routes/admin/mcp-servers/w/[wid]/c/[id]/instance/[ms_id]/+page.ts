import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { profile } = await parent();
	const workspaceId = params.wid;
	const catalogEntryId = params.id;
	const mcpServerId = params.ms_id;
	let mcpServer;
	let belongsToUser;

	let catalogEntry;
	try {
		catalogEntry = await UserService.getWorkspaceMCPCatalogEntry(workspaceId, catalogEntryId, {
			fetch
		});
		mcpServer = await UserService.getSingleOrRemoteMcpServer(mcpServerId, { fetch });
	} catch (err) {
		handleRouteError(
			err,
			`/admin/mcp-servers/w/${workspaceId}/c/${catalogEntryId}/instance/${mcpServerId}`,
			profile
		);
	}

	try {
		const userWorkspaceId = await UserService.fetchWorkspaceIDForProfile(profile.id, { fetch });
		belongsToUser = userWorkspaceId === workspaceId;
	} catch (_err) {
		belongsToUser = false;
	}

	return {
		workspaceId,
		catalogEntry,
		mcpServerId,
		mcpServer,
		belongsToUser
	};
};
