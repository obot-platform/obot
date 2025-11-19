import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const catalogEntryId = params.id;
	const mcpServerId = params.ms_id;
	let workspaceId;
	let catalogEntry;
	try {
		workspaceId = await ChatService.fetchWorkspaceIDForProfile(profile.current?.id, { fetch });
		catalogEntry = await ChatService.getWorkspaceMCPCatalogEntry(workspaceId, catalogEntryId, {
			fetch
		});
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
		mcpServerId
	};
};
