import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;
	let workspaceId;
	let catalogEntry;
	try {
		workspaceId = await ChatService.fetchWorkspaceIDForProfile(profile.current?.id, { fetch });
		catalogEntry = await ChatService.getWorkspaceMCPCatalogEntry(workspaceId, id, { fetch });
	} catch (err) {
		handleRouteError(err, `/mcp-hosting/c/${id}`, profile.current);
	}

	return {
		workspaceId,
		catalogEntry
	};
};
