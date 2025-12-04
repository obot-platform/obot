import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const workspaceId = params.wid;
	const catalogEntryId = params.id;
	const mcpServerId = params.ms_id;

	let catalogEntry;
	try {
		catalogEntry = await ChatService.getWorkspaceMCPCatalogEntry(workspaceId, catalogEntryId, {
			fetch
		});
	} catch (err) {
		handleRouteError(
			err,
			`/admin/mcp-servers/w/${workspaceId}/c/${catalogEntryId}/instance/${mcpServerId}`,
			profile.current
		);
	}

	return {
		workspaceId,
		catalogEntry,
		mcpServerId
	};
};
