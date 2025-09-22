import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id, wid } = params;
	let catalogEntry;
	try {
		catalogEntry = await ChatService.getWorkspaceMCPCatalogEntry(wid, id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/mcp-servers/w/${wid}/c/${id}`, profile.current);
	}
	return {
		workspaceId: wid,
		catalogEntry
	};
};
